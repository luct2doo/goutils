package response

import (
	"github.com/luct2doo/goutils/logger"
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// JSON 响应 200 和 JSON 数据
func JSON(c *gin.Context, data any) {
	c.JSON(http.StatusOK, data)
}

// Success 响应 200 和预设『操作成功！』的 JSON 数据
// 执行某个『没有具体返回数据』的『变更』操作成功后调用，例如删除、修改密码、修改手机号等。
func Success(c *gin.Context) {
	JSON(c, gin.H{
		"success": true,
		"message": "操作成功！",
		"code":    http.StatusOK,
	})
}

// Data 响应 200 和带 data 键的 JSON 数据
// 执行『更新操作』成功后调用，例如更新话题，成功后返回已更新的话题
func Data(c *gin.Context, data any) {
	JSON(c, gin.H{
		"success": true,
		"data":    data,
		"code":    http.StatusOK,
	})
}

// Created 响应 201 和带 data 键的 JSON 数据
// 执行『更新操作』成功后调用，例如更新话题，成功后返回已更新的话题
func Created(c *gin.Context, data any) {
	c.JSON(http.StatusCreated, gin.H{
		"success": true,
		"data":    data,
		"code":    http.StatusCreated,
	})
}

// CreatedJSON 响应 201 和 JSON 数据
func CreatedJSON(c *gin.Context, data any) {
	// 如果data是一个map或结构体，我们可以尝试添加code字段
	// 但由于data类型不确定，这里直接使用原始数据
	c.JSON(http.StatusCreated, data)
}

// Abort404 响应 404，未传参 msg 时使用默认消息
func Abort404(c *gin.Context, msg ...string) {
	c.AbortWithStatusJSON(http.StatusNotFound, gin.H{
		"message": defaultMessage("数据不存在，请确定请求正确", msg...),
		"code":    http.StatusNotFound,
	})
}

// Abort403 响应 403，未传参 msg 时使用默认消息
func Abort403(c *gin.Context, msg ...string) {
	c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
		"message": defaultMessage("权限不足，请确定您有对应的权限", msg...),
		"code":    http.StatusForbidden,
	})
}

// Abort409 响应 409
func Abort409(c *gin.Context, msg ...string) {
	c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
		"message": defaultMessage("服务器数据源有冲突", msg...),
		"code":    http.StatusConflict,
	})
}

// Abort500 响应 500，未传参 msg 时使用默认消息
func Abort500(c *gin.Context, msg ...string) {
	c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
		"message": defaultMessage("服务器内部错误，请稍后再试", msg...),
		"code":    http.StatusInternalServerError,
	})
}

// BadRequest 响应 400，传参 err 对象，未传参 msg 时使用默认消息
// 在解析用户请求，请求的格式或者方法不符合预期时调用
func BadRequest(c *gin.Context, err error, msg ...string) {
	logger.LogIf(err)
	c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
		"message": defaultMessage("请求解析错误，请确认请求格式是否正确。上传文件请使用 multipart 标头，参数请使用 JSON 格式。", msg...),
		"error":   err.Error(),
		"code":    http.StatusBadRequest,
	})
}

// Error 响应 404 或 422，未传参 msg 时使用默认消息
// 处理请求时出现错误 err，会附带返回 error 信息，如登录错误、找不到 ID 对应的 Model
func Error(c *gin.Context, err error, msg ...string) {
	logger.LogIf(err)

	// error 类型为『数据库未找到内容』
	if err == gorm.ErrRecordNotFound {
		Abort404(c)
		return
	}

	c.AbortWithStatusJSON(http.StatusUnprocessableEntity, gin.H{
		"message": defaultMessage("请求处理失败，请查看 error 的值", msg...),
		"error":   err.Error(),
		"code":    http.StatusUnprocessableEntity,
	})
}

// ValidationError 处理表单验证不通过的错误，返回的 JSON 示例：
//
//	 {
//	   "errors": {
//	      "phone": [
//	         "手机号为必填项，参数名称 phone",
//	         "手机号长度必须为 11 位的数字"
//	   		],
//	  },
//	  "message": "请求验证不通过，具体请查看 errors"
//	}
func ValidationError(c *gin.Context, errors map[string][]string) {
	c.AbortWithStatusJSON(http.StatusUnprocessableEntity, gin.H{
		"message": "请求验证不通过，具体请查看 errors",
		"errors":  errors,
		"code":    http.StatusUnprocessableEntity,
	})
}

// Unauthorized 响应 401，未传参 msg 时使用默认消息
// 登录失败、jwt 解析失败时调用
func Unauthorized(c *gin.Context, msg ...string) {
	c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
		"message": defaultMessage("认证失败，请确认您已登录或 Token 是否有效。", msg...),
		"code":    http.StatusUnauthorized,
	})
}

// defaultMessage 内用的辅助函数，用以支持默认参数默认值
// Go 不支持参数默认值，只能使用多变参数来实现类似效果
func defaultMessage(defaultMsg string, msg ...string) (message string) {
	if len(msg) > 0 {
		message = msg[0]
	} else {
		message = defaultMsg
	}
	return
}
