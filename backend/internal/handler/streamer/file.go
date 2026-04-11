package streamer

import (
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

func ServeFile(
	c *gin.Context,
	path string,
	contentType string,
	downloadName string,
) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open file: %w", err)
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return fmt.Errorf("stat file: %w", err)
	}
	if info.Size() <= 0 {
		return fmt.Errorf("empty file")
	}

	if strings.TrimSpace(contentType) != "" {
		c.Header("Content-Type", contentType)
	}
	if strings.TrimSpace(downloadName) != "" {
		c.Header("Content-Disposition", `attachment; filename="`+downloadName+`"`)
	}
	c.Header("Content-Length", strconv.FormatInt(info.Size(), 10))

	http.ServeContent(c.Writer, c.Request, info.Name(), info.ModTime(), file)
	return nil
}
