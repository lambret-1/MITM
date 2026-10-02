package include

import (
	"github.com/sagernet/sing-box/adapter/service"
	"github.com/sagernet/sing-box/protocol/mitm"
)

// registerMITMService 注册 MITM 服务到服务注册表
func registerMITMService(registry *service.Registry) {
	mitm.RegisterService(registry)
}
