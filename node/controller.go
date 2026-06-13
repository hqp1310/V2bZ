package node

import (
	"context"
	"fmt"

	panel "github.com/ZicBoard/ZicNode/api/zicboard"
	"github.com/ZicBoard/ZicNode/common/task"
	"github.com/ZicBoard/ZicNode/conf"
	"github.com/ZicBoard/ZicNode/core"
	"github.com/ZicBoard/ZicNode/limiter"
	log "github.com/sirupsen/logrus"
)

type Controller struct {
	server                  *core.V2Core
	apiClient               *panel.Client
	tag                     string
	limiter                 *limiter.Limiter
	userList                []panel.UserInfo
	aliveMap                map[int]int
	conf                    *conf.NodeConfig
	info                    *panel.NodeInfo
	nodeFingerprint         string
	nodeAdded               bool
	nodeInfoMonitorPeriodic *task.Task
	userReportPeriodic      *task.Task
	renewCertPeriodic       *task.Task
}

// NewController return a Node controller with default parameters.
func NewController(api *panel.Client, conf *conf.NodeConfig, info *panel.NodeInfo) *Controller {
	controller := &Controller{
		apiClient: api,
		info:      info,
		conf:      conf,
	}
	return controller
}

// Start implement the Start() function of the service interface
func (c *Controller) Start(x *core.V2Core) error {
	if err := c.Prepare(x); err != nil {
		return err
	}
	return c.StartPrepared(x)
}

func (c *Controller) Prepare(x *core.V2Core) error {
	c.server = x
	var err error
	node := c.info
	if node == nil {
		c.info, err = c.apiClient.GetNodeInfo(context.Background())
		if err != nil {
			return fmt.Errorf("get node info error: %s", err)
		}
		node = c.info
	}
	c.userList, err = c.apiClient.GetUserList(context.Background())
	if err != nil {
		return fmt.Errorf("get user list error: %s", err)
	}
	if len(c.userList) == 0 {
		log.WithField("tag", node.Tag).Warn("Started with empty user list; waiting for user sync")
	}
	c.aliveMap, err = c.apiClient.GetUserAlive(context.Background())
	if err != nil {
		return fmt.Errorf("failed to get user alive list: %s", err)
	}
	c.tag = node.Tag
	if node.Security == panel.Tls {
		err = c.requestCertAndReport(false)
		if err != nil {
			return fmt.Errorf("request cert error: %s", err)
		}
	}
	c.info = node
	c.nodeFingerprint = node.CoreFingerprint()
	return nil
}

func (c *Controller) StartPrepared(x *core.V2Core) error {
	c.server = x
	node := c.info
	if node == nil {
		return fmt.Errorf("node info is not prepared")
	}
	c.tag = node.Tag
	if c.userList == nil {
		c.userList = []panel.UserInfo{}
	}
	if c.aliveMap == nil {
		c.aliveMap = make(map[int]int)
	}

	l := limiter.AddLimiter(c.info.Type, c.tag, c.userList, c.aliveMap)
	c.limiter = l
	// Add new tag
	err := c.server.AddNode(c.tag, node)
	if err != nil {
		return fmt.Errorf("add new node error: %s", err)
	}
	c.nodeAdded = true
	added, err := c.server.AddUsers(&core.AddUsersParams{
		Tag:      c.tag,
		Users:    c.userList,
		NodeInfo: node,
	})
	if err != nil {
		return fmt.Errorf("add users error: %s", err)
	}
	log.WithField("tag", c.tag).Infof("Added %d new users", added)
	c.info = node
	if c.nodeFingerprint == "" {
		c.nodeFingerprint = node.CoreFingerprint()
	}
	c.startTasks(node)
	return nil
}

// Close implement the Close() function of the service interface
func (c *Controller) Close() error {
	if c.tag != "" {
		limiter.DeleteLimiter(c.tag)
	}
	if c.nodeInfoMonitorPeriodic != nil {
		c.nodeInfoMonitorPeriodic.Close()
		c.nodeInfoMonitorPeriodic = nil
	}
	if c.userReportPeriodic != nil {
		c.userReportPeriodic.Close()
		c.userReportPeriodic = nil
	}
	if c.renewCertPeriodic != nil {
		c.renewCertPeriodic.Close()
		c.renewCertPeriodic = nil
	}
	if c.nodeAdded && c.server != nil {
		err := c.server.DelNode(c.tag)
		if err != nil {
			return fmt.Errorf("del node error: %s", err)
		}
		c.nodeAdded = false
	}
	return nil
}
