// Task 10 replaces this file with real thread/attachment mounting.
package ticket

import (
	"context"

	"github.com/gin-gonic/gin"
	"github.com/grandpine/ticket-api/internal/apperr"
	"github.com/grandpine/ticket-api/internal/db"
)

func attachFiles(_ context.Context, _ *db.Queries, _ int64, fileIDs []int64) error {
	if len(fileIDs) > 0 {
		return apperr.Validation("file_ids", "attachments are not supported yet")
	}
	return nil
}

func (h *Handler) mountThread(private *gin.RouterGroup) {}
