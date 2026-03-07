package server

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsIPInTrustedSubnet(t *testing.T) {
	tests := []struct {
		name     string
		ip       string
		subnet   string
		expected bool
	}{
		{
			name:     "valid ip in subnet",
			ip:       "192.168.1.100",
			subnet:   "192.168.1.0/24",
			expected: true,
		},
		{
			name:     "valid ip not in subnet",
			ip:       "10.0.0.1",
			subnet:   "192.168.1.0/24",
			expected: false,
		},
		{
			name:     "invalid ip format",
			ip:       "invalid",
			subnet:   "192.168.1.0/24",
			expected: false,
		},
		{
			name:     "invalid subnet format",
			ip:       "192.168.1.100",
			subnet:   "invalid",
			expected: false,
		},
		{
			name:     "empty subnet",
			ip:       "192.168.1.100",
			subnet:   "",
			expected: false,
		},
		{
			name:     "ip exactly at network boundary",
			ip:       "192.168.1.0",
			subnet:   "192.168.1.0/24",
			expected: true,
		},
		{
			name:     "ip exactly at broadcast",
			ip:       "192.168.1.255",
			subnet:   "192.168.1.0/24",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isIPInTrustedSubnet(tt.ip, tt.subnet)
			assert.Equal(t, tt.expected, result)
		})
	}
}
