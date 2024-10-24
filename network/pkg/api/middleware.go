package api

import (
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// CORS middleware
func corsMiddleware(allowedOrigins []string) gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")

		// Check if origin is allowed
		allowed := false
		for _, allowedOrigin := range allowedOrigins {
			if allowedOrigin == "*" || allowedOrigin == origin {
				allowed = true
				break
			}
		}

		if allowed {
			c.Writer.Header().Set("Access-Control-Allow-Origin", origin)
			c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
			c.Writer.Header().Set("Access-Control-Max-Age", "86400")
		}

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(200)
			return
		}

		c.Next()
	}
}

// Logger middleware
func loggerMiddleware(logger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		query := c.Request.URL.RawQuery

		c.Next()

		end := time.Now()
		latency := end.Sub(start)

		if len(c.Errors) > 0 {
			// Log errors
			for _, e := range c.Errors.Errors() {
				logger.Error("Request error",
					zap.String("path", path),
					zap.String("query", query),
					zap.String("method", c.Request.Method),
					zap.Int("status", c.Writer.Status()),
					zap.Duration("latency", latency),
					zap.String("error", e),
				)
			}
		} else {
			// Log successful requests
			logger.Info("Request processed",
				zap.String("path", path),
				zap.String("query", query),
				zap.String("method", c.Request.Method),
				zap.Int("status", c.Writer.Status()),
				zap.Duration("latency", latency),
			)
		}
	}
}

// Recovery middleware
func recoveryMiddleware(logger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if err := recover(); err != nil {
				logger.Error("Request panic recovered",
					zap.Any("error", err),
					zap.String("path", c.Request.URL.Path),
				)

				c.AbortWithStatusJSON(500, APIError{
					Code:    500,
					Message: "Internal server error",
				})
			}
		}()

		c.Next()
	}
}

// Authentication middleware
func authMiddleware(authType AuthType) gin.HandlerFunc {
	return func(c *gin.Context) {
		token := c.GetHeader("Authorization")

		if token == "" {
			c.AbortWithStatusJSON(401, APIError{
				Code:    401,
				Message: "Authorization header is required",
			})
			return
		}

		// Verify token based on auth type
		valid, err := verifyToken(token, authType)
		if err != nil || !valid {
			c.AbortWithStatusJSON(401, APIError{
				Code:    401,
				Message: "Invalid authorization token",
			})
			return
		}

		c.Next()
	}
}

// Rate limiter middleware
func rateLimiterMiddleware(limit int, window time.Duration) gin.HandlerFunc {
	type clientLimit struct {
		count    int
		lastSeen time.Time
	}

	limits := make(map[string]*clientLimit)

	return func(c *gin.Context) {
		clientIP := c.ClientIP()
		now := time.Now()

		if client, exists := limits[clientIP]; exists {
			if now.Sub(client.lastSeen) > window {
				// Reset counter if window has passed
				client.count = 0
				client.lastSeen = now
			}

			if client.count >= limit {
				c.AbortWithStatusJSON(429, APIError{
					Code:    429,
					Message: "Rate limit exceeded",
				})
				return
			}

			client.count++
		} else {
			limits[clientIP] = &clientLimit{
				count:    1,
				lastSeen: now,
			}
		}

		c.Next()
	}
}

// Request validation middleware
func validationMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Validate content type for POST/PUT requests
		if c.Request.Method == "POST" || c.Request.Method == "PUT" {
			contentType := c.GetHeader("Content-Type")
			if contentType != "application/json" {
				c.AbortWithStatusJSON(415, APIError{
					Code:    415,
					Message: "Content-Type must be application/json",
				})
				return
			}
		}

		// Validate required parameters
		if err := validateRequiredParams(c); err != nil {
			c.AbortWithStatusJSON(400, APIError{
				Code:    400,
				Message: err.Error(),
			})
			return
		}

		c.Next()
	}
}

// Metrics middleware
func metricsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()

		// Add request to metrics
		incrementRequestCount(c.Request.Method, c.Request.URL.Path)

		c.Next()

		// Record response time
		duration := time.Since(start)
		recordResponseTime(c.Request.Method, c.Request.URL.Path, duration)

		// Record response status
		recordResponseStatus(c.Writer.Status())
	}
}

// Helper types and functions

type AuthType int

const (
	AuthTypeBearer AuthType = iota
	AuthTypeAPIKey
	AuthTypeJWT
)

func verifyToken(token string, authType AuthType) (bool, error) {
	switch authType {
	case AuthTypeBearer:
		return verifyBearerToken(token)
	case AuthTypeAPIKey:
		return verifyAPIKey(token)
	case AuthTypeJWT:
		return verifyJWT(token)
	default:
		return false, fmt.Errorf("unsupported auth type")
	}
}

func validateRequiredParams(c *gin.Context) error {
	// Example validation - implement based on your needs
	return nil
}

// Metrics helpers
func incrementRequestCount(method, path string) {
	// Implement metric tracking
}

func recordResponseTime(method, path string, duration time.Duration) {
	// Implement metric tracking
}

func recordResponseStatus(status int) {
	// Implement metric tracking
}
