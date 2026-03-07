package middleware

import (
	"net"
	"net/http"
	"strings"

	"github.com/IvanDolgov/go-musthave-metrics-tpl/internal/logger"
	"go.uber.org/zap"
)

// TrustedSubnetMiddleware проверяет, что IP-адрес клиента входит в доверенную подсеть
func TrustedSubnetMiddleware(trustedSubnet string) func(http.Handler) http.Handler {
	// Парсим доверенную подсеть ОДИН РАЗ при создании middleware
	var ipNet *net.IPNet
	if trustedSubnet != "" {
		_, parsed, err := net.ParseCIDR(trustedSubnet)
		if err != nil {
			logger.Log.Error("Invalid trusted subnet CIDR",
				zap.String("subnet", trustedSubnet),
				zap.Error(err))
			// Возвращаем middleware, который будет отклонять все запросы
			return func(next http.Handler) http.Handler {
				return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					http.Error(w, "Invalid trusted subnet configuration", http.StatusInternalServerError)
				})
			}
		}
		ipNet = parsed
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Если подсеть не задана, пропускаем все запросы
			if trustedSubnet == "" || ipNet == nil {
				next.ServeHTTP(w, r)
				return
			}

			// Получаем IP из заголовка X-Real-IP
			realIP := r.Header.Get("X-Real-IP")
			if realIP == "" {
				logger.Log.Warn("Missing X-Real-IP header",
					zap.String("remote_addr", r.RemoteAddr))
				http.Error(w, "Forbidden", http.StatusForbidden)
				return
			}

			// Парсим IP адрес
			ip := net.ParseIP(realIP)
			if ip == nil {
				logger.Log.Warn("Invalid IP in X-Real-IP header",
					zap.String("x-real-ip", realIP))
				http.Error(w, "Forbidden", http.StatusForbidden)
				return
			}

			// Проверяем, входит ли IP в доверенную подсеть
			if !ipNet.Contains(ip) {
				logger.Log.Warn("IP not in trusted subnet",
					zap.String("ip", realIP),
					zap.String("trusted_subnet", trustedSubnet))
				http.Error(w, "Forbidden", http.StatusForbidden)
				return
			}

			// IP в доверенной подсети, пропускаем запрос
			next.ServeHTTP(w, r)
		})
	}
}

// GetRealIP извлекает реальный IP клиента из заголовков
func GetRealIP(r *http.Request) string {
	// Пробуем получить из X-Real-IP
	if realIP := r.Header.Get("X-Real-IP"); realIP != "" {
		return realIP
	}

	// Пробуем получить из X-Forwarded-For
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		// Берём первый IP в списке
		parts := strings.Split(forwarded, ",")
		if len(parts) > 0 {
			return strings.TrimSpace(parts[0])
		}
	}

	// Возвращаем RemoteAddr как запасной вариант
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}
