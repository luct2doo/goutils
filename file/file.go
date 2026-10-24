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
	// SaveUploadImgCompressed 与 SaveUploadImg 相同，但落盘后会把过大的 JPG/PNG
	// 等比缩小到 maxEdge 像素内并重新编码。用于手机直出的资质照片。
	SaveUploadImgCompressed(c *gin.Context, file *multipart.FileHeader, dest string, maxEdge, quality int) (string, error)
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

// SaveUploadImgCompressed 保存图片并压缩。
//
// 手机直出的照片常是 4,000×3,000 / 3~8MB，一张资质照片就这样存下来，
// 一单 18 份材料就是上百兆。这里按「长边 ≤ maxEdge」等比缩小，JPG 再以
// quality 重编码；PNG 保留无损格式，只做尺寸收缩。
//
// 压缩失败不回滚：原图已经落盘，能打开比报错更有价值；压缩后的文件
// 只有在比原图更小时才替换，避免小图被重编码后反而变大。
func (fs *fileService) SaveUploadImgCompressed(c *gin.Context, file *multipart.FileHeader, dest string, maxEdge, quality int) (string, error) {
	dirName, fullPath, err := fs.saveUpload(c, file, dest)
	if err != nil {
		return "", err
	}
	// 默认值兜底：调用方传 0 时不至于把图片缩成 1px。
	if maxEdge <= 0 {
		maxEdge = 2400
	}
	if quality <= 0 || quality > 100 {
		quality = 85
	}
	if err := compressUploadedImage(fullPath, maxEdge, quality); err != nil {
		// 不是图片 / 扩展名伪装 / 解码失败：保留原文件，交给后续流程判断。
		return fs.app.URL(dirName + filepath.Base(fullPath)), nil
	}
	return fs.app.URL(dirName + filepath.Base(fullPath)), nil
}

// compressUploadedImage 就地把过大的 JPG/PNG 缩小并重编码。
//
// 只处理 .jpg/.jpeg/.png：WebP 在 imaging 里只有解码器、没有编码器，
// 强行转成 JPEG 会改扩展名，反而让库里路径和实际格式不一致。
func compressUploadedImage(fullPath string, maxEdge, quality int) error {
	ext := strings.ToLower(filepath.Ext(fullPath))
	if ext != ".jpg" && ext != ".jpeg" && ext != ".png" {
		return nil
	}

	before, err := os.Stat(fullPath)
	if err != nil {
		return err
	}

	img, err := imaging.Open(fullPath, imaging.AutoOrientation(true))
	if err != nil {
		return err
	}

	bounds := img.Bounds()
	tooLarge := bounds.Dx() > maxEdge || bounds.Dy() > maxEdge
	// 已经很小、也不大的图，重编码只会掉画质。
	if !tooLarge && before.Size() <= 1<<20 {
		return nil
	}
	if tooLarge {
		img = imaging.Fit(img, maxEdge, maxEdge, imaging.Lanczos)
	}

	// imaging.Save 按扩展名选编码器：临时文件名必须保留 .jpg/.png 结尾，
	// 否则会报 "unsupported image format"。
	tmpPath := fullPath + ".compressed" + ext
	if ext == ".png" {
		err = imaging.Save(img, tmpPath)
	} else {
		err = imaging.Save(img, tmpPath, imaging.JPEGQuality(quality))
	}
	if err != nil {
		_ = os.Remove(tmpPath)
		return err
	}

	after, err := os.Stat(tmpPath)
	if err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	if after.Size() >= before.Size() {
		_ = os.Remove(tmpPath)
		return nil
	}
	return os.Rename(tmpPath, fullPath)
}
