package domain_test

import (
	"testing"

	"cdpnetool/pkg/domain"
)

func TestNormalizeResourceType(t *testing.T) {
	tests := []struct {
		name       string
		cdpType    string
		url        string
		wantType   domain.ResourceType
		wantReason string
	}{
		// CDP 标准类型直接映射
		{
			name:       "CDP Document type",
			cdpType:    "Document",
			url:        "https://example.com/index.html",
			wantType:   domain.ResourceTypeDocument,
			wantReason: "CDP Document 应该映射为 document",
		},
		{
			name:       "CDP Script type",
			cdpType:    "Script",
			url:        "https://example.com/app.js",
			wantType:   domain.ResourceTypeScript,
			wantReason: "CDP Script 应该映射为 script",
		},
		{
			name:       "CDP XHR type",
			cdpType:    "XHR",
			url:        "https://api.example.com/data",
			wantType:   domain.ResourceTypeXHR,
			wantReason: "CDP XHR 应该映射为 xhr",
		},

		// Fetch 类型通过 URL 扩展名智能归类
		{
			name:       "Fetch loading .js file",
			cdpType:    "Fetch",
			url:        "https://example.com/bundle.js",
			wantType:   domain.ResourceTypeScript,
			wantReason: "Fetch 加载 .js 文件应该归类为 script",
		},
		{
			name:       "Fetch loading .mjs file",
			cdpType:    "Fetch",
			url:        "https://example.com/module.mjs",
			wantType:   domain.ResourceTypeScript,
			wantReason: "Fetch 加载 .mjs 文件应该归类为 script",
		},
		{
			name:       "Fetch loading .css file",
			cdpType:    "Fetch",
			url:        "https://example.com/style.css",
			wantType:   domain.ResourceTypeStylesheet,
			wantReason: "Fetch 加载 .css 文件应该归类为 stylesheet",
		},
		{
			name:       "Fetch loading API",
			cdpType:    "Fetch",
			url:        "https://api.example.com/v1/users",
			wantType:   domain.ResourceTypeFetch,
			wantReason: "Fetch 加载 API 应该保持为 fetch",
		},

		// Other 类型通过 URL 扩展名智能归类
		{
			name:       "Other with .js extension",
			cdpType:    "Other",
			url:        "https://example.com/vendor.js",
			wantType:   domain.ResourceTypeScript,
			wantReason: "Other 类型但有 .js 扩展名应该归类为 script",
		},
		{
			name:       "Other with .png extension",
			cdpType:    "Other",
			url:        "https://example.com/logo.png",
			wantType:   domain.ResourceTypeImage,
			wantReason: "Other 类型但有 .png 扩展名应该归类为 image",
		},

		// URL 带查询参数
		{
			name:       "JS file with query params",
			cdpType:    "Fetch",
			url:        "https://example.com/app.js?v=1.0.0",
			wantType:   domain.ResourceTypeScript,
			wantReason: "带查询参数的 .js 文件应该归类为 script",
		},
		{
			name:       "CSS file with hash",
			cdpType:    "Fetch",
			url:        "https://example.com/style.css#section",
			wantType:   domain.ResourceTypeStylesheet,
			wantReason: "带哈希的 .css 文件应该归类为 stylesheet",
		},

		// 其他资源类型
		{
			name:       "Font file (.woff2)",
			cdpType:    "Fetch",
			url:        "https://example.com/fonts/roboto.woff2",
			wantType:   domain.ResourceTypeFont,
			wantReason: ".woff2 文件应该归类为 font",
		},
		{
			name:       "Image file (.svg)",
			cdpType:    "Other",
			url:        "https://example.com/icon.svg",
			wantType:   domain.ResourceTypeImage,
			wantReason: ".svg 文件应该归类为 image",
		},
		{
			name:       "Media file (.mp4)",
			cdpType:    "Fetch",
			url:        "https://example.com/video.mp4",
			wantType:   domain.ResourceTypeMedia,
			wantReason: ".mp4 文件应该归类为 media",
		},

		// 大小写不敏感
		{
			name:       "CDP type uppercase",
			cdpType:    "DOCUMENT",
			url:        "https://example.com/",
			wantType:   domain.ResourceTypeDocument,
			wantReason: "CDP 类型应该大小写不敏感",
		},
		{
			name:       "URL extension uppercase",
			cdpType:    "Fetch",
			url:        "https://example.com/APP.JS",
			wantType:   domain.ResourceTypeScript,
			wantReason: "URL 扩展名应该大小写不敏感",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := domain.NormalizeResourceType(tt.cdpType, tt.url)
			if got != tt.wantType {
				t.Errorf("NormalizeResourceType(%q, %q) = %q, want %q\nReason: %s",
					tt.cdpType, tt.url, got, tt.wantType, tt.wantReason)
			}
		})
	}
}

func TestGuessTypeFromURL(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		wantType domain.ResourceType
	}{
		// JavaScript 扩展名（只测试支持的扩展名）
		{"js file", "https://example.com/app.js", domain.ResourceTypeScript},
		{"mjs file", "https://example.com/module.mjs", domain.ResourceTypeScript},

		// CSS 扩展名
		{"css file", "https://example.com/style.css", domain.ResourceTypeStylesheet},

		// 图片扩展名（只测试支持的扩展名）
		{"png file", "https://example.com/image.png", domain.ResourceTypeImage},
		{"jpg file", "https://example.com/photo.jpg", domain.ResourceTypeImage},
		{"jpeg file", "https://example.com/photo.jpeg", domain.ResourceTypeImage},
		{"gif file", "https://example.com/anim.gif", domain.ResourceTypeImage},
		{"svg file", "https://example.com/icon.svg", domain.ResourceTypeImage},
		{"webp file", "https://example.com/image.webp", domain.ResourceTypeImage},

		// 字体扩展名（只测试支持的扩展名）
		{"woff file", "https://example.com/font.woff", domain.ResourceTypeFont},
		{"woff2 file", "https://example.com/font.woff2", domain.ResourceTypeFont},
		{"ttf file", "https://example.com/font.ttf", domain.ResourceTypeFont},

		// 音视频扩展名（只测试支持的扩展名）
		{"mp4 file", "https://example.com/video.mp4", domain.ResourceTypeMedia},
		{"mp3 file", "https://example.com/audio.mp3", domain.ResourceTypeMedia},

		// 无法识别（不常见扩展名或特殊类型）
		// 注意：通过 NormalizeResourceType("", url) 调用时，
		// URL 无法识别会返回 Other（因为 CDP 类型为空会被归类为 Other）
		{"jsx file", "https://example.com/component.jsx", domain.ResourceTypeOther},
		{"ts file", "https://example.com/app.ts", domain.ResourceTypeOther},
		{"tsx file", "https://example.com/component.tsx", domain.ResourceTypeOther},
		{"scss file", "https://example.com/style.scss", domain.ResourceTypeOther},
		{"sass file", "https://example.com/style.sass", domain.ResourceTypeOther},
		{"less file", "https://example.com/style.less", domain.ResourceTypeOther},
		{"otf file", "https://example.com/font.otf", domain.ResourceTypeOther},
		{"eot file", "https://example.com/font.eot", domain.ResourceTypeOther},
		{"webm file", "https://example.com/video.webm", domain.ResourceTypeOther},
		{"wav file", "https://example.com/audio.wav", domain.ResourceTypeOther},
		{"api endpoint", "https://api.example.com/v1/users", domain.ResourceTypeOther},
		{"no extension", "https://example.com/path/to/resource", domain.ResourceTypeOther},
		{"webmanifest file", "https://example.com/manifest.webmanifest", domain.ResourceTypeOther},
		{"manifest.json file", "https://example.com/manifest.json", domain.ResourceTypeOther},
		{"vtt file", "https://example.com/subtitles.vtt", domain.ResourceTypeOther},
		{"srt file", "https://example.com/subtitles.srt", domain.ResourceTypeOther},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 注意：guessTypeFromURL 是内部函数，不能直接调用
			// 我们通过 NormalizeResourceType 使用空 CDP 类型来间接测试
			got := domain.NormalizeResourceType("", tt.url)
			if got != tt.wantType {
				t.Errorf("guessTypeFromURL(%q) = %q, want %q", tt.url, got, tt.wantType)
			}
		})
	}
}
