package executor

// RequestMutation 请求修改结果
type RequestMutation struct {
	URL           *string
	Method        *string
	Headers       map[string]string
	RemoveHeaders []string
	Query         map[string]string
	RemoveQuery   []string
	Cookies       map[string]string
	RemoveCookies []string
	Body          *string
	Block         *BlockResponse // 终结性行为
}

// BlockResponse 拦截响应
type BlockResponse struct {
	StatusCode int
	Headers    map[string]string
	Body       []byte
}

// ResponseMutation 响应修改结果
type ResponseMutation struct {
	StatusCode    *int
	Headers       map[string]string
	RemoveHeaders []string
	Body          *string
}
