package remotetools

import (
	"github.com/kira1928/remotetools/pkg/tools"
	"github.com/kira1928/remotetools/pkg/version"
)

// Version 是库的当前版本号
const Version = version.Version

// Get 返回工具管理 API 实例
func Get() *tools.API {
	return tools.Get()
}
