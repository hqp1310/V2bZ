package node

import (
	"context"
	"fmt"

	panel "github.com/ZicBoard/ZicNode/api/zicboard"
	"github.com/ZicBoard/ZicNode/conf"
	"github.com/ZicBoard/ZicNode/core"
	log "github.com/sirupsen/logrus"
)

type Node struct {
	controllers []*Controller
	NodeInfos   []*panel.NodeInfo
}

func New(nodes []conf.NodeConfig) (*Node, error) {
	n := &Node{
		controllers: make([]*Controller, len(nodes)),
		NodeInfos:   make([]*panel.NodeInfo, len(nodes)),
	}
	for i, node := range nodes {
		p, err := panel.New(&node)
		if err != nil {
			return nil, err
		}
		info, err := p.GetNodeInfo(context.Background())
		if err != nil {
			return nil, err
		}
		n.controllers[i] = NewController(p, &node, info)
		n.NodeInfos[i] = info
	}
	return n, nil
}

func (n *Node) Start(nodes []conf.NodeConfig, core *core.V2Core) error {
	for i, node := range nodes {
		err := n.controllers[i].Start(core)
		if err != nil {
			return fmt.Errorf("start node controller [%s-%d] error: %s",
				node.APIHost,
				node.NodeID,
				err)
		}
	}
	n.refreshNodeInfos()
	return nil
}

func (n *Node) Prepare(core *core.V2Core) error {
	for i, c := range n.controllers {
		if err := c.Prepare(core); err != nil {
			return fmt.Errorf("prepare node controller [%s-%d] error: %s",
				c.conf.APIHost,
				c.conf.NodeID,
				err)
		}
		n.NodeInfos[i] = cloneNodeInfo(c.info)
	}
	return nil
}

func (n *Node) StartPrepared(core *core.V2Core) error {
	for i, c := range n.controllers {
		err := c.StartPrepared(core)
		if err != nil {
			return fmt.Errorf("start prepared node controller [%s-%d] error: %s",
				c.conf.APIHost,
				c.conf.NodeID,
				err)
		}
		n.NodeInfos[i] = cloneNodeInfo(c.info)
	}
	return nil
}

func (n *Node) Close() error {
	var err error
	for _, c := range n.controllers {
		if c == nil {
			continue
		}
		if closeErr := c.Close(); closeErr != nil {
			log.Errorf("close controller failed: %v", closeErr)
			if err == nil {
				err = closeErr
			}
		}
	}
	n.controllers = nil
	return err
}

func (n *Node) refreshNodeInfos() {
	if n == nil {
		return
	}
	for i, c := range n.controllers {
		if c != nil {
			n.NodeInfos[i] = cloneNodeInfo(c.info)
		}
	}
}
