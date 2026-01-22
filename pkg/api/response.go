package api

// Response 是所有 GUI 前后端通信的统一响应格式
type Response[T any] struct {
	Success bool   `json:"success"`
	Code    string `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
	Data    T      `json:"data,omitempty"`
}

// OK 构造成功响应
func OK[T any](data T) Response[T] {
	return Response[T]{
		Success: true,
		Data:    data,
	}
}

// Fail 构造失败响应
func Fail[T any](code, message string) Response[T] {
	return Response[T]{
		Success: false,
		Code:    code,
		Message: message,
	}
}

// EmptyData 用于无业务数据返回的场景
type EmptyData struct{}
