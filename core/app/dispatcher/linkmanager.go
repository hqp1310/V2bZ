package dispatcher

import (
	"sync"

	log "github.com/sirupsen/logrus"
	"github.com/xtls/xray-core/common"
	"github.com/xtls/xray-core/common/buf"
)

type ManagedWriter struct {
	writer  buf.Writer
	manager *LinkManager
}

func (w *ManagedWriter) WriteMultiBuffer(mb buf.MultiBuffer) error {
	return w.writer.WriteMultiBuffer(mb)
}

func (w *ManagedWriter) Close() error {
	w.manager.RemoveWriter(w)
	return common.Close(w.writer)
}

type LinkManager struct {
	links map[*ManagedWriter]buf.Reader
	mu    sync.RWMutex
}

func (m *LinkManager) AddLink(writer *ManagedWriter, reader buf.Reader) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.links[writer] = reader
}

func (m *LinkManager) RemoveWriter(writer *ManagedWriter) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.links, writer)
}

func (m *LinkManager) Count() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.links)
}

func (m *LinkManager) CloseAll(reason string, fields log.Fields) int {
	m.mu.Lock()
	links := m.links
	m.links = make(map[*ManagedWriter]buf.Reader)
	m.mu.Unlock()

	closed := len(links)
	for w, r := range links {
		common.Close(w)
		common.Interrupt(r)
	}

	if closed > 0 {
		logFields := cloneLogFields(fields)
		logFields["event"] = "zicnode_disconnect"
		logFields["reason"] = reason
		logFields["action"] = "close_managed_links"
		logFields["closed_links"] = closed
		log.WithFields(logFields).Error("managed links closed")
	}
	return closed
}

func cloneLogFields(fields log.Fields) log.Fields {
	cloned := log.Fields{}
	for key, value := range fields {
		cloned[key] = value
	}
	return cloned
}
