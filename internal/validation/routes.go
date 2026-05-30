package validation

import (
	"fmt"
	"net"
)

// RouteConfig defines a single route configuration.
type RouteConfig struct {
	Target string `json:"target"` // Target URL
	Port   int    `json:"port"`   // Listening port
}

// ValidateRoutes validates the routes configuration.
func ValidateRoutes(routes []RouteConfig) error {
	if len(routes) == 0 {
		return nil
	}

	// Check for duplicate ports
	portSet := make(map[int]bool)

	for i, route := range routes {
		// Validate target URL
		if err := ValidateURL(route.Target); err != nil {
			return fmt.Errorf("route[%d]: invalid target: %v", i, err)
		}

		// Validate port range
		if route.Port < 1 || route.Port > 65535 {
			return fmt.Errorf("route[%d]: invalid port %d", i, route.Port)
		}

		// Check for duplicate ports
		if portSet[route.Port] {
			return fmt.Errorf("route[%d]: port %d is duplicated", i, route.Port)
		}
		portSet[route.Port] = true

		// Check if port is available in the system
		if err := CheckPortAvailable(route.Port); err != nil {
			return fmt.Errorf("route[%d]: %v", i, err)
		}
	}

	return nil
}

// CheckPortAvailable checks if a port is available.
func CheckPortAvailable(port int) error {
	addr := fmt.Sprintf(":%d", port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("port %d is already in use", port)
	}
	listener.Close()
	return nil
}
