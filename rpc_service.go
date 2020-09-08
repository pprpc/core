package pprpc

import (
	"sync"

	"github.com/pprpc/packets"
)

type cmdHandler func(interface{}, RPCConn, *packets.CmdPacket, bool, func(interface{}) error) (interface{}, error)

// ServiceDesc 存放接口调用描述
type ServiceDesc struct {
	CmdID       uint64
	CmdName     string
	ReqHandler  cmdHandler
	RespHandler cmdHandler
	Hanlder     interface{}
}

// Service 定义支持的服务
type Service struct {
	cmds *sync.Map
}

// NewService 创建服务
func NewService() *Service {
	s := new(Service)
	s.cmds = new(sync.Map)
	return s
}

// RegisterService 注册服务
func (s *Service) RegisterService(d *ServiceDesc, h interface{}) {
	d.Hanlder = h
	s.cmds.Store(d.CmdID, d)
}

// UnRegService 取消注册服务
func (s *Service) UnRegService(id uint64) {
	s.cmds.Delete(id)
}

// Length 获取注册服务长度
func (s *Service) Length() int {
	return 0
}

// GetService 获取一个Service
func (s *Service) GetService(id uint64) *ServiceDesc {
	v, ok := s.cmds.Load(id)
	if ok {
		return v.(*ServiceDesc)
	}
	return nil
}

// GetAllService 获取所有的Service
func (s *Service) GetAllService() []*ServiceDesc {
	return nil
}
