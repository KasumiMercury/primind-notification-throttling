package middleware

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
)

func PanicRecoveryGin() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if rec := recover(); rec != nil {
				ctx := c.Request.Context()

				slog.ErrorContext(ctx, "panic recovered",
					slog.String("event", "app.panic"),
					slog.Any("error", rec),
				)

				c.AbortWithStatus(http.StatusInternalServerError)

				// Re-panic
				panic(rec)
			}
		}()

		c.Next()
	}
}
