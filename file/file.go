package file

import (
	"fmt"
	"mime/multipart"
	"os"
	"path/filepath"
	"strings"

	"github.com/luct2doo/goutils/app"
	"github.com/luct2doo/goutils/helpers"

	"github.com/disintegration/imaging"
	"github.com/gin-gonic/gin"
)

// DefaultPublicRoot 默认的落盘根目录（相对进程工作目录，不是绝对路径）。
// 各项目的静态资源目录不同，建议在 NewFileService 时显式传入。
const DefaultPublicRoot = "public"

// UIDFunc 从 gin.Context 中提取「当前用户 ID」，用于把上传文件按用户分目录存放。
//
// 库本身不认识任何应用的鉴权上下文（例如 rugao 的 auth.CurrentUID），
// 因此由调用方注入。传 nil 时统一落到 anonymous 目录。
type UIDFunc func(c *gin.Context) string

type FileService interface {
	Put(data []byte, to string) error
	Exists(fileToCheck string) bool
	FileNameWithoutExtension(fileName string) string
	SaveUploadAvatar(c *gin.Context, file *multipart.FileHeader) (string, error)
	SaveUploadImg(c *gin.Context, file *multipart.FileHeader, dest string) (string, error)
	RandomNameFromUploadFile(file *multipart.FileHeader) string
}

// fileService 实现 FileService 接口
type fileService struct {
	app     *app.App
	root    string  // 上传文件落盘根目录
	uidFunc UIDFunc // 当前用户 ID 提取函数，可为 nil
}

// NewFileService 创建文件服务实例
// 参数:
//   - app: 应用程序实例，通过依赖注入获取
//   - root: 上传文件落盘根目录（如 "../../public"），空字符串则用 DefaultPublicRoot
//   - uidFunc: 从上下文提取当前用户 ID 的函数，可为 nil（则按 anonymous 分目录）
//
// 返回:
//   - FileService: 文件服务接口实例
func NewFileService(app *app.App, root string, uidFunc UIDFunc) FileService {
	if root == "" {
		root = DefaultPublicRoot
	}
	return &fileService{
		app:     app,
		root:    root,
		uidFunc: uidFunc,
	}
}

// Put 写入文件到指定路径
// 参数:
//   - data: 要写入的文件数据
//   - to: 目标文件路径
//
// 返回:
//   - error: 如果写入失败，返回错误；否则返回 nil
func (fs *fileService) Put(data []byte, to string) error {
	return os.WriteFile(to, data, 0o644)
}

// Exists 检查文件是否存在
// 参数:
//   - fileToCheck: 要检查的文件路径
//
// 返回:
//   - bool: 如果文件存在，返回 true；否则返回 false
func (fs *fileService) Exists(fileToCheck string) bool {
	if _, err := os.Stat(fileToCheck); os.IsNotExist(err) {
		return false
	}
	return true
}

// FileNameWithoutExtension 获取文件名（不包含扩展名）
// 参数:
//   - fileName: 包含扩展名的文件名
//
// 返回:
//   - string: 文件名（不包含扩展名）
func (fs *fileService) FileNameWithoutExtension(fileName string) string {
	return strings.TrimSuffix(fileName, filepath.Ext(fileName))
}

// currentUID 取当前用户 ID，无 uidFunc 或取不到时返回 "anonymous"
func (fs *fileService) currentUID(c *gin.Context) string {
	if fs.uidFunc == nil {
		return "anonymous"
	}
	if uid := fs.uidFunc(c); uid != "" {
		return uid
	}
	return "anonymous"
}

// saveUpload 将上传文件保存到 {root}/uploads/{dest}/{日期}/{用户ID}/ 目录下，
// 返回 web 相对目录 dirName（含末尾斜杠）与本地完整路径 fullPath。
func (fs *fileService) saveUpload(c *gin.Context, file *multipart.FileHeader, dest string) (dirName, fullPath string, err error) {
	dirName = fmt.Sprintf("/uploads/%s/%s/%s/", dest, fs.app.TimeNowInTimezone().Format("2006/01/02"), fs.currentUID(c))
	if err = os.MkdirAll(fs.root+dirName, 0o755); err != nil {
		return "", "", err
	}

	fileName := fs.RandomNameFromUploadFile(file)
	fullPath = fs.root + dirName + fileName
	if err = c.SaveUploadedFile(file, fullPath); err != nil {
		return "", "", err
	}
	return dirName, fullPath, nil
}

func (fs *fileService) SaveUploadAvatar(c *gin.Context, file *multipart.FileHeader) (string, error) {
	// 保存原始文件
	dirName, fullPath, err := fs.saveUpload(c, file, "avatars")
	if err != nil {
		return "", err
	}

	// 裁切图片
	img, err := imaging.Open(fullPath, imaging.AutoOrientation(true))
	if err != nil {
		return "", err
	}
	resizeAvatar := imaging.Thumbnail(img, 256, 256, imaging.Lanczos)
	resizeAvatarName := fs.RandomNameFromUploadFile(file)
	resizeAvatarPath := filepath.Dir(fullPath) + "/" + resizeAvatarName
	if err := imaging.Save(resizeAvatar, resizeAvatarPath); err != nil {
		return "", err
	}

	// 删除老文件
	if err := os.Remove(fullPath); err != nil {
		return "", err
	}

	return dirName + resizeAvatarName, nil
}

func (fs *fileService) RandomNameFromUploadFile(file *multipart.FileHeader) string {
	return helpers.RandomString(16) + filepath.Ext(file.Filename)
}

func (fs *fileService) SaveUploadImg(c *gin.Context, file *multipart.FileHeader, dest string) (string, error) {
	dirName, fullPath, err := fs.saveUpload(c, file, dest)
	if err != nil {
		return "", err
	}
	return fs.app.URL(dirName + filepath.Base(fullPath)), nil
}
