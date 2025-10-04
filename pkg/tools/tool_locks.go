package tools

import (
	"errors"
	"sync"
)

// ErrToolBusy 表示同一工具目录已有其他操作持有锁
var ErrToolBusy = errors.New("tool is busy: another operation is in progress")

// tryLocker 是一个支持 TryLock/Unlock 的简化接口
type tryLocker interface {
	TryLock() bool
	Unlock()
}

// go1.18+ 的 sync.Mutex 原生支持 TryLock；封装以便将来替换实现
type tryMutex struct{ sync.Mutex }

func (m *tryMutex) TryLock() bool { return m.Mutex.TryLock() }

// 全局锁映射：按工具安装目录（toolFolder）划分，确保同一工具的关键操作互斥
var toolLocks sync.Map // map[string]*tryMutex

func getToolMutex(toolFolder string) tryLocker {
	if m, ok := toolLocks.Load(toolFolder); ok {
		return m.(*tryMutex)
	}
	m := &tryMutex{}
	actual, _ := toolLocks.LoadOrStore(toolFolder, m)
	return actual.(*tryMutex)
}
