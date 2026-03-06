package server

import (
	"net"

	"github.com/IvanDolgov/go-musthave-metrics-tpl/internal/logger"
	"go.uber.org/zap"
)

// isIPInTrustedSubnet проверяет, входит ли IP в доверенную подсеть
func isIPInTrustedSubnet(ipStr, subnetStr string) bool {
	if subnetStr == "" {
		return true
	}

	_, ipNet, err := net.ParseCIDR(subnetStr)
	if err != nil {
		logger.Log.Error("Invalid trusted subnet CIDR",
			zap.String("subnet", subnetStr),
			zap.Error(err))
		return false
	}

	ip := net.ParseIP(ipStr)
	if ip == nil {
		logger.Log.Warn("Invalid IP address", zap.String("ip", ipStr))
		return false
	}

	return ipNet.Contains(ip)
}
