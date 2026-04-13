package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/gin-gonic/gin"
)

type artifactEntry struct {
	Name string `json:"name"`
	Path string `json:"path"`
	Kind string `json:"kind"`
}

func (h Handlers) getJobArtifact(c *gin.Context) {
	if h.jobReader == nil {
		failure(c, http.StatusInternalServerError, 5003, "任务查询服务未初始化")
		return
	}
	if strings.TrimSpace(h.workspace) == "" {
		failure(c, http.StatusInternalServerError, 5003, "artifact 服务未初始化")
		return
	}

	jobID := c.Param("job_id")
	job, err := h.jobReader.GetJobByPublicID(c.Request.Context(), jobID)
	if err != nil {
		if isJobNotFound(err) {
			failure(c, http.StatusNotFound, 1002, "任务不存在")
			return
		}
		failure(c, http.StatusInternalServerError, 5003, "查询任务失败")
		return
	}

	relativePath, err := validateJobArtifactPath(job.PublicID, c.Query("path"))
	if err != nil {
		failure(c, http.StatusBadRequest, 1001, err.Error())
		return
	}

	fullPath := filepath.Join(h.workspace, filepath.Clean(relativePath))
	info, err := os.Stat(fullPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			failure(c, http.StatusNotFound, 1005, "artifact 不存在")
			return
		}
		failure(c, http.StatusInternalServerError, 5003, "读取 artifact 失败")
		return
	}

	if info.IsDir() {
		entries, err := listArtifactEntries(fullPath, relativePath)
		if err != nil {
			failure(c, http.StatusInternalServerError, 5003, "读取 artifact 目录失败")
			return
		}
		success(c, http.StatusOK, gin.H{
			"path":    relativePath,
			"kind":    "directory",
			"entries": entries,
		})
		return
	}

	kind, contentType, ok := detectArtifactKind(relativePath)
	if !ok {
		failure(c, http.StatusBadRequest, 1001, "artifact 只支持 json/md/txt/srt")
		return
	}

	data, err := os.ReadFile(fullPath)
	if err != nil {
		failure(c, http.StatusInternalServerError, 5003, "读取 artifact 失败")
		return
	}

	response := gin.H{
		"path":         relativePath,
		"kind":         kind,
		"content_type": contentType,
	}

	if kind == "json" {
		var payload any
		if err := json.Unmarshal(data, &payload); err != nil {
			failure(c, http.StatusInternalServerError, 5003, "artifact JSON 解析失败")
			return
		}
		response["json"] = payload
	} else {
		response["text"] = string(data)
	}

	success(c, http.StatusOK, response)
}

func validateJobArtifactPath(jobPublicID string, rawPath string) (string, error) {
	trimmed := strings.TrimSpace(rawPath)
	if trimmed == "" {
		return "", errInvalidArticle("path 不能为空")
	}
	if filepath.IsAbs(trimmed) {
		return "", errInvalidArticle("path 必须是相对路径")
	}

	clean := filepath.ToSlash(filepath.Clean(trimmed))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", errInvalidArticle("path 非法")
	}

	jobRoot := filepath.ToSlash(filepath.Join("jobs", jobPublicID))
	if clean != jobRoot && !strings.HasPrefix(clean, jobRoot+"/") {
		return "", errInvalidArticle("path 必须属于当前 job")
	}

	return clean, nil
}

func detectArtifactKind(path string) (string, string, bool) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".json":
		return "json", "application/json", true
	case ".md":
		return "text", "text/markdown; charset=utf-8", true
	case ".txt":
		return "text", "text/plain; charset=utf-8", true
	case ".srt":
		return "text", "text/plain; charset=utf-8", true
	default:
		return "", "", false
	}
}

func listArtifactEntries(fullPath string, relativePath string) ([]artifactEntry, error) {
	entries, err := os.ReadDir(fullPath)
	if err != nil {
		return nil, err
	}

	items := make([]artifactEntry, 0, len(entries))
	for _, entry := range entries {
		entryPath := filepath.ToSlash(filepath.Join(relativePath, entry.Name()))
		if entry.IsDir() {
			items = append(items, artifactEntry{
				Name: entry.Name(),
				Path: entryPath,
				Kind: "directory",
			})
			continue
		}

		kind, _, ok := detectArtifactKind(entryPath)
		if !ok {
			continue
		}
		items = append(items, artifactEntry{
			Name: entry.Name(),
			Path: entryPath,
			Kind: kind,
		})
	}

	sort.Slice(items, func(i, j int) bool {
		if items[i].Kind != items[j].Kind {
			return items[i].Kind == "directory"
		}
		return items[i].Name < items[j].Name
	})

	return items, nil
}
