package middleware

import (
	"fmt"
	"net/http"

	httpx "github.com/prawirdani/golang-restapi/internal/transport/http"
	"github.com/prawirdani/golang-restapi/pkg/log"
)

func PanicRecoverer(next httpx.Func) httpx.Func {
	return func(c *httpx.Context) error {
		defer func() {
			if rec := recover(); rec != nil {
				err := fmt.Errorf("%v", rec)
				log.ErrorCtx(c.Context(), "panic recovered",
					err,
					"path", c.URLPath(),
					"method", c.Method(),
				)
				if err := c.JSON(http.StatusInternalServerError, &httpx.Body{
					Message: "Something went wrong, try again later!",
				}); err != nil {
					http.Error(c.Writer(), "internal server error", http.StatusInternalServerError)
				}

			}
		}()

		return next(c)
	}
}
