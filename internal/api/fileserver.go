// author: weiqiang; date: 2025-11
package api

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/spf13/viper"
)

const (
	MaxFileSize = 100 << 20 // 100MB
)

// FileInfo 文件信息结构体
type FileInfo struct {
	FileName    string    `json:"file_name"`
	FileSize    int64     `json:"file_size"`
	UploadTime  time.Time `json:"upload_time"`
	DownloadURL string    `json:"download_url"`
}

// FileServerRoute /api/v1/file/ 文件下载及上传管理路由
func FileServerRoute(r *gin.RouterGroup) {

	UploadDir := viper.GetString("fileserver.path")

	// 确保上传目录存在
	if err := os.MkdirAll(UploadDir, 0755); err != nil {
		panic(fmt.Sprintf("创建上传目录失败: %v", err))
	}

	// 健康检查
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status": "ok",
			"time":   time.Now().Format(time.RFC3339),
		})
	})

	// 添加CORS中间件（可选，用于跨域请求）
	r.Use(corsMiddleware())
	// 静态文件服务，用于下载文件
	r.Static("/files", UploadDir)

	// 上传文件
	r.POST("/upload", func(c *gin.Context) {
		uploadFile(c, UploadDir)
	})

	// 获取文件列表
	r.GET("/files", func(c *gin.Context) {
		listFiles(c, UploadDir)
	})

	// 下载文件
	r.GET("/download/:filename", func(c *gin.Context) {
		downloadFile(c, UploadDir)
	})

	// 删除文件
	r.DELETE("/file/:filename", func(c *gin.Context) {
		deleteFile(c, UploadDir)
	})

	// 首页
	r.GET("/", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"message": "文件服务器已启动",
			"apis": map[string]string{
				"上传文件": "POST /api/v1/file/upload",
				"文件列表": "GET /api/v1/file/files",
				"下载文件": "GET /api/v1/file/download/:filename",
				"删除文件": "DELETE /api/v1/file/file/:filename",
				"直接下载": "GET /files/:filename",
			},
		})
	})
}

// CORS中间件
func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	}
}

// 上传文件处理函数
func uploadFile(c *gin.Context, UploadDir string) {
	// 限制请求体大小
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, MaxFileSize)

	// 解析表单
	if err := c.Request.ParseMultipartForm(MaxFileSize); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "文件太大",
			"message": fmt.Sprintf("文件大小不能超过 %dMB", MaxFileSize/(1<<20)),
		})
		return
	}

	// 获取文件
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "无效的文件",
			"message": err.Error(),
		})
		return
	}
	defer file.Close()

	// 检查文件名安全性
	filename := filepath.Base(header.Filename)
	if filename == "." || filename == ".." {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "无效的文件名",
			"message": "文件名不能为 '.' 或 '..'",
		})
		return
	}

	// 创建目标文件
	filePath := filepath.Join(UploadDir, filename)
	dst, err := os.Create(filePath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "创建文件失败",
			"message": err.Error(),
		})
		return
	}
	defer dst.Close()

	// 复制文件内容
	if _, err := io.Copy(dst, file); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "保存文件失败",
			"message": err.Error(),
		})
		return
	}

	// 返回成功响应
	fileInfo := FileInfo{
		FileName:    filename,
		FileSize:    header.Size,
		UploadTime:  time.Now(),
		DownloadURL: fmt.Sprintf("/api/v1/file/download/%s", filename),
	}

	c.JSON(http.StatusOK, gin.H{
		"message":    "文件上传成功",
		"file_info":  fileInfo,
		"direct_url": fmt.Sprintf("/files/%s", filename),
	})
}

// 获取文件列表
func listFiles(c *gin.Context, UploadDir string) {
	entries, err := os.ReadDir(UploadDir)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "读取文件列表失败",
			"message": err.Error(),
		})
		return
	}

	var files []FileInfo
	for _, entry := range entries {
		if !entry.IsDir() {
			info, err := entry.Info()
			if err != nil {
				continue
			}

			files = append(files, FileInfo{
				FileName:    entry.Name(),
				FileSize:    info.Size(),
				UploadTime:  info.ModTime(),
				DownloadURL: fmt.Sprintf("/api/v1/file/download/%s", entry.Name()),
			})
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"files": files,
		"count": len(files),
	})
}

// 下载文件处理函数
func downloadFile(c *gin.Context, UploadDir string) {
	filename := c.Param("filename")

	// 检查文件名安全性
	filename = filepath.Base(filename)
	if filename == "." || filename == ".." {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "无效的文件名",
		})
		return
	}

	filePath := filepath.Join(UploadDir, filename)

	// 检查文件是否存在
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		c.JSON(http.StatusNotFound, gin.H{
			"error":   "文件不存在",
			"message": fmt.Sprintf("文件 '%s' 未找到", filename),
		})
		return
	}

	// 设置下载头
	c.Header("Content-Description", "File Transfer")
	c.Header("Content-Transfer-Encoding", "binary")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))
	c.Header("Content-Type", "application/octet-stream")

	// 提供文件下载
	c.File(filePath)
}

// 删除文件
func deleteFile(c *gin.Context, UploadDir string) {
	filename := c.Param("filename")

	// 检查文件名安全性
	filename = filepath.Base(filename)
	if filename == "." || filename == ".." {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "无效的文件名",
		})
		return
	}

	filePath := filepath.Join(UploadDir, filename)

	// 检查文件是否存在
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		c.JSON(http.StatusNotFound, gin.H{
			"error":   "文件不存在",
			"message": fmt.Sprintf("文件 '%s' 未找到", filename),
		})
		return
	}

	// 删除文件
	if err := os.Remove(filePath); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "删除文件失败",
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": fmt.Sprintf("文件 '%s' 删除成功", filename),
	})
}
