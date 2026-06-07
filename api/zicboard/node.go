package panel

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Security type
const (
	None    = 0
	Tls     = 1
	Reality = 2
)

type NodeInfo struct {
	Id           int
	Type         string
	Security     int
	PushInterval time.Duration
	PullInterval time.Duration
	Tag          string
	Common       *CommonNode
}

type CommonNode struct {
	Protocol   string      `json:"protocol"`
	Host       string      `json:"host"`
	ListenIP   string      `json:"listen_ip"`
	ServerPort int         `json:"server_port"`
	Routes     []Route     `json:"routes"`
	BaseConfig *BaseConfig `json:"base_config"`
	//vless vmess trojan
	Tls                int         `json:"tls"`
	TlsSettings        TlsSettings `json:"tls_settings"`
	CertInfo           *CertInfo
	Network            string          `json:"network"`
	NetworkSettings    json.RawMessage `json:"network_settings"`
	Encryption         string          `json:"encryption"`
	EncryptionSettings EncSettings     `json:"encryption_settings"`
	ServerName         string          `json:"server_name"`
	Flow               string          `json:"flow"`
	//shadowsocks
	Cipher    string `json:"cipher"`
	ServerKey string `json:"server_key"`
	//tuic
	CongestionControl string `json:"congestion_control"`
	ZeroRTTHandshake  bool   `json:"zero_rtt_handshake"`
	//anytls
	PaddingScheme []string `json:"padding_scheme,omitempty"`
	//warp outbound
	WarpSettings WarpSettings `json:"warp_settings"`
	//hysteria hysteria2
	UpMbps                  int    `json:"up_mbps"`
	DownMbps                int    `json:"down_mbps"`
	Obfs                    string `json:"obfs"`
	ObfsPassword            string `json:"obfs_password"`
	Ignore_Client_Bandwidth bool   `json:"ignore_client_bandwidth"`
}

type Route struct {
	Id          int      `json:"id"`
	Match       []string `json:"match"`
	Action      string   `json:"action"`
	ActionValue *string  `json:"action_value"`
}

type BaseConfig struct {
	Panel                  string `json:"panel"`
	NodeType               string `json:"node_type"`
	PushInterval           any    `json:"push_interval"`
	PullInterval           any    `json:"pull_interval"`
	DeviceOnlineMinTraffic int    `json:"device_online_min_traffic"`
	NodeReportMinTraffic   int    `json:"node_report_min_traffic"`
}

type WarpSettings struct {
	Enable         bool     `json:"enable"`
	Mode           string   `json:"mode"`
	FailPolicy     string   `json:"fail_policy"`
	MTU            int      `json:"mtu"`
	DomainStrategy string   `json:"domain_strategy"`
	PrivateKey     string   `json:"private_key"`
	PeerPublicKey  string   `json:"peer_public_key"`
	Endpoint       string   `json:"endpoint"`
	Addresses      []string `json:"addresses"`
	Reserved       []byte   `json:"reserved"`
}

type TlsSettings struct {
	ServerName       string   `json:"server_name"`
	ServerNames      []string `json:"server_names"`
	Dest             string   `json:"dest"`
	ServerPort       string   `json:"server_port"`
	ShortId          string   `json:"short_id"`
	ShortIds         []string `json:"short_ids"`
	PrivateKey       string   `json:"private_key"`
	Mldsa65Seed      string   `json:"mldsa65Seed"`
	Xver             uint64   `json:"xver"`
	CertMode         string   `json:"cert_mode"`
	CertFile         string   `json:"cert_file"`
	KeyFile          string   `json:"key_file"`
	Provider         string   `json:"provider"`
	DNSEnv           string   `json:"dns_env"`
	SelfFallback     bool     `json:"self_fallback"`
	RejectUnknownSni string   `json:"reject_unknown_sni"`
}

type CertInfo struct {
	CertMode         string
	CertFile         string
	KeyFile          string
	Email            string
	CertDomain       string
	DNSEnv           map[string]string
	Provider         string
	SelfFallback     bool
	RejectUnknownSni bool
}

type EncSettings struct {
	Mode          string `json:"mode"`
	Ticket        string `json:"ticket"`
	ServerPadding string `json:"server_padding"`
	PrivateKey    string `json:"private_key"`
}

func (c *CommonNode) UnmarshalJSON(data []byte) error {
	if jsonRawIsEmpty(data) {
		*c = CommonNode{}
		return nil
	}

	var raw struct {
		Protocol              json.RawMessage `json:"protocol"`
		Host                  json.RawMessage `json:"host"`
		ListenIP              json.RawMessage `json:"listen_ip"`
		ServerPort            json.RawMessage `json:"server_port"`
		Routes                []Route         `json:"routes"`
		BaseConfig            *BaseConfig     `json:"base_config"`
		Tls                   json.RawMessage `json:"tls"`
		TlsSettings           json.RawMessage `json:"tls_settings"`
		Network               json.RawMessage `json:"network"`
		NetworkSettings       json.RawMessage `json:"network_settings"`
		Encryption            json.RawMessage `json:"encryption"`
		EncryptionSettings    json.RawMessage `json:"encryption_settings"`
		ServerName            json.RawMessage `json:"server_name"`
		Flow                  json.RawMessage `json:"flow"`
		Cipher                json.RawMessage `json:"cipher"`
		ServerKey             json.RawMessage `json:"server_key"`
		CongestionControl     json.RawMessage `json:"congestion_control"`
		ZeroRTTHandshake      json.RawMessage `json:"zero_rtt_handshake"`
		PaddingScheme         json.RawMessage `json:"padding_scheme"`
		WarpSettings          json.RawMessage `json:"warp_settings"`
		UpMbps                json.RawMessage `json:"up_mbps"`
		DownMbps              json.RawMessage `json:"down_mbps"`
		Obfs                  json.RawMessage `json:"obfs"`
		ObfsPassword          json.RawMessage `json:"obfs_password"`
		IgnoreClientBandwidth json.RawMessage `json:"ignore_client_bandwidth"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	var tlsSettings TlsSettings
	if err := unmarshalFlexibleObject(raw.TlsSettings, &tlsSettings); err != nil {
		return fmt.Errorf("decode tls_settings error: %s", err)
	}
	var encryptionSettings EncSettings
	if err := unmarshalFlexibleObject(raw.EncryptionSettings, &encryptionSettings); err != nil {
		return fmt.Errorf("decode encryption_settings error: %s", err)
	}
	var warpSettings WarpSettings
	if err := unmarshalFlexibleObject(raw.WarpSettings, &warpSettings); err != nil {
		return fmt.Errorf("decode warp_settings error: %s", err)
	}

	*c = CommonNode{
		Protocol:                flexibleString(raw.Protocol),
		Host:                    flexibleString(raw.Host),
		ListenIP:                flexibleString(raw.ListenIP),
		ServerPort:              flexibleInt(raw.ServerPort),
		Routes:                  raw.Routes,
		BaseConfig:              raw.BaseConfig,
		Tls:                     flexibleInt(raw.Tls),
		TlsSettings:             tlsSettings,
		Network:                 flexibleString(raw.Network),
		NetworkSettings:         jsonRawOrNil(raw.NetworkSettings),
		Encryption:              flexibleString(raw.Encryption),
		EncryptionSettings:      encryptionSettings,
		ServerName:              flexibleString(raw.ServerName),
		Flow:                    flexibleString(raw.Flow),
		Cipher:                  flexibleString(raw.Cipher),
		ServerKey:               flexibleString(raw.ServerKey),
		CongestionControl:       flexibleString(raw.CongestionControl),
		ZeroRTTHandshake:        flexibleBool(raw.ZeroRTTHandshake),
		PaddingScheme:           flexibleStringSlice(raw.PaddingScheme),
		WarpSettings:            warpSettings,
		UpMbps:                  flexibleInt(raw.UpMbps),
		DownMbps:                flexibleInt(raw.DownMbps),
		Obfs:                    flexibleString(raw.Obfs),
		ObfsPassword:            flexibleString(raw.ObfsPassword),
		Ignore_Client_Bandwidth: flexibleBool(raw.IgnoreClientBandwidth),
	}
	return nil
}

func (r *Route) UnmarshalJSON(data []byte) error {
	if jsonRawIsEmpty(data) {
		*r = Route{}
		return nil
	}

	var raw struct {
		Id          json.RawMessage `json:"id"`
		Match       json.RawMessage `json:"match"`
		Action      json.RawMessage `json:"action"`
		ActionValue json.RawMessage `json:"action_value"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	r.Id = flexibleInt(raw.Id)
	r.Match = flexibleStringSlice(raw.Match)
	r.Action = flexibleString(raw.Action)
	if jsonRawIsEmpty(raw.ActionValue) {
		r.ActionValue = nil
	} else {
		value := flexibleString(raw.ActionValue)
		r.ActionValue = &value
	}
	return nil
}

func (b *BaseConfig) UnmarshalJSON(data []byte) error {
	if jsonRawIsEmpty(data) {
		*b = BaseConfig{}
		return nil
	}

	var raw struct {
		Panel                  json.RawMessage `json:"panel"`
		NodeType               json.RawMessage `json:"node_type"`
		PushInterval           any             `json:"push_interval"`
		PullInterval           any             `json:"pull_interval"`
		DeviceOnlineMinTraffic json.RawMessage `json:"device_online_min_traffic"`
		NodeReportMinTraffic   json.RawMessage `json:"node_report_min_traffic"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	*b = BaseConfig{
		Panel:                  flexibleString(raw.Panel),
		NodeType:               flexibleString(raw.NodeType),
		PushInterval:           raw.PushInterval,
		PullInterval:           raw.PullInterval,
		DeviceOnlineMinTraffic: flexibleInt(raw.DeviceOnlineMinTraffic),
		NodeReportMinTraffic:   flexibleInt(raw.NodeReportMinTraffic),
	}
	return nil
}

func (w *WarpSettings) UnmarshalJSON(data []byte) error {
	if jsonRawIsEmpty(data) {
		*w = WarpSettings{}
		return nil
	}

	var raw struct {
		Enable         json.RawMessage `json:"enable"`
		Mode           json.RawMessage `json:"mode"`
		FailPolicy     json.RawMessage `json:"fail_policy"`
		MTU            json.RawMessage `json:"mtu"`
		DomainStrategy json.RawMessage `json:"domain_strategy"`
		PrivateKey     json.RawMessage `json:"private_key"`
		PeerPublicKey  json.RawMessage `json:"peer_public_key"`
		Endpoint       json.RawMessage `json:"endpoint"`
		Addresses      json.RawMessage `json:"addresses"`
		Reserved       json.RawMessage `json:"reserved"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	mode := strings.ToLower(strings.TrimSpace(flexibleString(raw.Mode)))
	if mode != "manual" {
		mode = "auto"
	}
	failPolicy := strings.ToLower(strings.TrimSpace(flexibleString(raw.FailPolicy)))
	if failPolicy != "block" {
		failPolicy = "direct"
	}
	mtu := flexibleInt(raw.MTU)
	if mtu <= 0 {
		mtu = 1280
	}
	domainStrategy := strings.TrimSpace(flexibleString(raw.DomainStrategy))
	if !isWarpDomainStrategy(domainStrategy) {
		domainStrategy = "ForceIPv4v6"
	}
	endpoint := strings.TrimSpace(flexibleString(raw.Endpoint))
	if endpoint == "" {
		endpoint = "engage.cloudflareclient.com:2408"
	}
	addresses := flexibleCSVStringSlice(raw.Addresses)
	if len(addresses) == 0 {
		addresses = []string{"172.16.0.2/32", "2606:4700:110:8765::2/128"}
	}

	*w = WarpSettings{
		Enable:         flexibleBool(raw.Enable),
		Mode:           mode,
		FailPolicy:     failPolicy,
		MTU:            mtu,
		DomainStrategy: domainStrategy,
		PrivateKey:     strings.TrimSpace(flexibleString(raw.PrivateKey)),
		PeerPublicKey:  strings.TrimSpace(flexibleString(raw.PeerPublicKey)),
		Endpoint:       endpoint,
		Addresses:      addresses,
		Reserved:       flexibleByteSlice(raw.Reserved),
	}
	return nil
}

func isWarpDomainStrategy(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "forceip", "forceipv4", "forceipv6", "forceipv4v6", "forceipv6v4":
		return true
	default:
		return false
	}
}

func (t *TlsSettings) UnmarshalJSON(data []byte) error {
	if jsonRawIsEmpty(data) {
		*t = TlsSettings{}
		return nil
	}

	var raw struct {
		ServerName       json.RawMessage `json:"server_name"`
		ServerNames      json.RawMessage `json:"server_names"`
		Dest             json.RawMessage `json:"dest"`
		ServerPort       json.RawMessage `json:"server_port"`
		ShortId          json.RawMessage `json:"short_id"`
		ShortIds         json.RawMessage `json:"short_ids"`
		PrivateKey       json.RawMessage `json:"private_key"`
		Mldsa65Seed      json.RawMessage `json:"mldsa65Seed"`
		Xver             json.RawMessage `json:"xver"`
		CertMode         json.RawMessage `json:"cert_mode"`
		CertFile         json.RawMessage `json:"cert_file"`
		KeyFile          json.RawMessage `json:"key_file"`
		Provider         json.RawMessage `json:"provider"`
		DNSEnv           json.RawMessage `json:"dns_env"`
		SelfFallback     json.RawMessage `json:"self_fallback"`
		RejectUnknownSni json.RawMessage `json:"reject_unknown_sni"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	*t = TlsSettings{
		ServerName:       flexibleString(raw.ServerName),
		ServerNames:      flexibleStringSlice(raw.ServerNames),
		Dest:             flexibleString(raw.Dest),
		ServerPort:       flexibleString(raw.ServerPort),
		ShortId:          flexibleString(raw.ShortId),
		ShortIds:         flexibleStringSlice(raw.ShortIds),
		PrivateKey:       flexibleString(raw.PrivateKey),
		Mldsa65Seed:      flexibleString(raw.Mldsa65Seed),
		Xver:             flexibleUint64(raw.Xver),
		CertMode:         flexibleString(raw.CertMode),
		CertFile:         flexibleString(raw.CertFile),
		KeyFile:          flexibleString(raw.KeyFile),
		Provider:         flexibleString(raw.Provider),
		DNSEnv:           flexibleString(raw.DNSEnv),
		SelfFallback:     flexibleBool(raw.SelfFallback),
		RejectUnknownSni: flexibleBoolString(raw.RejectUnknownSni),
	}
	return nil
}

func (e *EncSettings) UnmarshalJSON(data []byte) error {
	if jsonRawIsEmpty(data) {
		*e = EncSettings{}
		return nil
	}

	var raw struct {
		Mode          json.RawMessage `json:"mode"`
		Ticket        json.RawMessage `json:"ticket"`
		ServerPadding json.RawMessage `json:"server_padding"`
		PrivateKey    json.RawMessage `json:"private_key"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	*e = EncSettings{
		Mode:          flexibleString(raw.Mode),
		Ticket:        flexibleString(raw.Ticket),
		ServerPadding: flexibleString(raw.ServerPadding),
		PrivateKey:    flexibleString(raw.PrivateKey),
	}
	return nil
}

func unmarshalFlexibleObject(raw json.RawMessage, target any) error {
	if jsonRawIsEmpty(raw) {
		return nil
	}
	trimmed := bytes.TrimSpace(raw)
	if bytes.Equal(trimmed, []byte("[]")) {
		return nil
	}
	if len(trimmed) > 0 && trimmed[0] == '"' {
		var encoded string
		if err := json.Unmarshal(trimmed, &encoded); err != nil {
			return err
		}
		encoded = strings.TrimSpace(encoded)
		if encoded == "" {
			return nil
		}
		trimmed = []byte(encoded)
	}
	return json.Unmarshal(trimmed, target)
}

func jsonRawIsEmpty(raw []byte) bool {
	trimmed := bytes.TrimSpace(raw)
	return len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null"))
}

func jsonRawOrNil(raw json.RawMessage) json.RawMessage {
	if jsonRawIsEmpty(raw) {
		return nil
	}
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) > 0 && trimmed[0] == '"' {
		var encoded string
		if err := json.Unmarshal(trimmed, &encoded); err == nil {
			encoded = strings.TrimSpace(encoded)
			if encoded == "" || strings.EqualFold(encoded, "null") {
				return nil
			}
			trimmed = []byte(encoded)
		}
	}
	return append(json.RawMessage(nil), trimmed...)
}

func flexibleString(raw []byte) string {
	if jsonRawIsEmpty(raw) {
		return ""
	}
	trimmed := bytes.TrimSpace(raw)

	var value string
	if err := json.Unmarshal(trimmed, &value); err == nil {
		return value
	}

	var compact bytes.Buffer
	if err := json.Compact(&compact, trimmed); err == nil {
		return compact.String()
	}
	return string(trimmed)
}

func flexibleStringSlice(raw []byte) []string {
	if jsonRawIsEmpty(raw) {
		return nil
	}
	trimmed := bytes.TrimSpace(raw)

	var items []json.RawMessage
	if err := json.Unmarshal(trimmed, &items); err == nil {
		return rawMessagesToStrings(items)
	}

	var value string
	if err := json.Unmarshal(trimmed, &value); err == nil {
		return stringToStringSlice(value)
	}

	value = flexibleString(trimmed)
	if value == "" {
		return nil
	}
	return []string{value}
}

func flexibleCSVStringSlice(raw []byte) []string {
	items := flexibleStringSlice(raw)
	if len(items) == 0 {
		return nil
	}
	values := make([]string, 0, len(items))
	for _, item := range items {
		for _, part := range strings.FieldsFunc(item, func(r rune) bool {
			return r == ',' || r == '\n' || r == '\r'
		}) {
			part = strings.TrimSpace(part)
			if part != "" {
				values = append(values, part)
			}
		}
	}
	return values
}

func flexibleByteSlice(raw []byte) []byte {
	items := flexibleCSVStringSlice(raw)
	if len(items) == 0 {
		return nil
	}
	values := make([]byte, 0, len(items))
	for _, item := range items {
		value := parseIntString(item)
		if value >= 0 && value <= 255 {
			values = append(values, byte(value))
		}
	}
	return values
}

func stringToStringSlice(value string) []string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" || strings.EqualFold(trimmed, "null") {
		return nil
	}
	if strings.HasPrefix(trimmed, "[") {
		var items []json.RawMessage
		if err := json.Unmarshal([]byte(trimmed), &items); err == nil {
			return rawMessagesToStrings(items)
		}
	}
	return []string{trimmed}
}

func rawMessagesToStrings(items []json.RawMessage) []string {
	values := make([]string, 0, len(items))
	for _, item := range items {
		value := flexibleString(item)
		if value != "" {
			values = append(values, value)
		}
	}
	return values
}

func flexibleInt(raw []byte) int {
	if jsonRawIsEmpty(raw) {
		return 0
	}
	trimmed := bytes.TrimSpace(raw)

	var value int
	if err := json.Unmarshal(trimmed, &value); err == nil {
		return value
	}

	var text string
	if err := json.Unmarshal(trimmed, &text); err == nil {
		return parseIntString(text)
	}

	var boolean bool
	if err := json.Unmarshal(trimmed, &boolean); err == nil {
		if boolean {
			return 1
		}
		return 0
	}

	var number float64
	if err := json.Unmarshal(trimmed, &number); err == nil {
		return int(number)
	}
	return 0
}

func flexibleUint64(raw []byte) uint64 {
	if jsonRawIsEmpty(raw) {
		return 0
	}
	trimmed := bytes.TrimSpace(raw)

	var value uint64
	if err := json.Unmarshal(trimmed, &value); err == nil {
		return value
	}

	var text string
	if err := json.Unmarshal(trimmed, &text); err == nil {
		return parseUint64String(text)
	}

	var boolean bool
	if err := json.Unmarshal(trimmed, &boolean); err == nil {
		if boolean {
			return 1
		}
		return 0
	}

	var number float64
	if err := json.Unmarshal(trimmed, &number); err == nil && number > 0 {
		return uint64(number)
	}
	return 0
}

func flexibleBool(raw []byte) bool {
	if jsonRawIsEmpty(raw) {
		return false
	}
	trimmed := bytes.TrimSpace(raw)

	var value bool
	if err := json.Unmarshal(trimmed, &value); err == nil {
		return value
	}

	var text string
	if err := json.Unmarshal(trimmed, &text); err == nil {
		return parseBoolString(text)
	}

	var number float64
	if err := json.Unmarshal(trimmed, &number); err == nil {
		return number != 0
	}
	return false
}

func flexibleBoolString(raw []byte) string {
	if jsonRawIsEmpty(raw) {
		return ""
	}
	value := strings.TrimSpace(flexibleString(raw))
	switch strings.ToLower(value) {
	case "1", "true", "yes", "y", "on":
		return "1"
	case "0", "false", "no", "n", "off":
		return "0"
	case "":
		return ""
	}

	if number, err := strconv.ParseFloat(value, 64); err == nil {
		if number != 0 {
			return "1"
		}
		return "0"
	}
	return value
}

func parseBoolString(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "y", "on":
		return true
	default:
		return false
	}
}

func parseIntString(value string) int {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return 0
	}
	if intValue, err := strconv.Atoi(trimmed); err == nil {
		return intValue
	}
	if floatValue, err := strconv.ParseFloat(trimmed, 64); err == nil {
		return int(floatValue)
	}
	if parseBoolString(trimmed) {
		return 1
	}
	return 0
}

func parseUint64String(value string) uint64 {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return 0
	}
	if uintValue, err := strconv.ParseUint(trimmed, 10, 64); err == nil {
		return uintValue
	}
	if floatValue, err := strconv.ParseFloat(trimmed, 64); err == nil && floatValue > 0 {
		return uint64(floatValue)
	}
	if parseBoolString(trimmed) {
		return 1
	}
	return 0
}

func flexibleIntFromAny(value any) int {
	switch typed := value.(type) {
	case nil:
		return 0
	case int:
		return typed
	case int8:
		return int(typed)
	case int16:
		return int(typed)
	case int32:
		return int(typed)
	case int64:
		return int(typed)
	case uint:
		return int(typed)
	case uint8:
		return int(typed)
	case uint16:
		return int(typed)
	case uint32:
		return int(typed)
	case uint64:
		return int(typed)
	case float32:
		return int(typed)
	case float64:
		return int(typed)
	case string:
		return parseIntString(typed)
	case bool:
		if typed {
			return 1
		}
		return 0
	case json.Number:
		return parseIntString(typed.String())
	default:
		raw, err := json.Marshal(typed)
		if err != nil {
			return 0
		}
		return flexibleInt(raw)
	}
}

func (c *Client) GetNodeInfo(ctx context.Context) (node *NodeInfo, err error) {
	const path = "/api/v3/server/config"
	r, err := c.client.
		R().
		SetContext(ctx).
		SetHeader("If-None-Match", c.nodeEtag).
		ForceContentType("application/json").
		Get(path)
	if err != nil {
		return nil, err
	}
	if r == nil {
		return nil, fmt.Errorf("received nil response")
	}

	if r.StatusCode() == 304 {
		return nil, nil
	}
	hash := sha256.Sum256(r.Body())
	newBodyHash := hex.EncodeToString(hash[:])
	if c.responseBodyHash == newBodyHash {
		return nil, nil
	}
	c.responseBodyHash = newBodyHash
	c.nodeEtag = r.Header().Get("ETag")

	if r != nil {
		defer func() {
			if r.RawBody() != nil {
				r.RawBody().Close()
			}
		}()
	} else {
		return nil, fmt.Errorf("received nil response")
	}
	node = &NodeInfo{
		Id: c.NodeId,
	}
	// parse protocol params
	cm := &CommonNode{}
	err = json.Unmarshal(r.Body(), cm)
	if err != nil {
		return nil, fmt.Errorf("decode node params error: %s", err)
	}
	if cm.BaseConfig == nil {
		return nil, fmt.Errorf("missing base_config from ZicBoard")
	}
	if cm.BaseConfig.Panel != "zicboard" {
		return nil, fmt.Errorf("unsupported panel %q: ZicNode only connects to ZicBoard", cm.BaseConfig.Panel)
	}
	if cm.BaseConfig.NodeType != "zicnode" {
		return nil, fmt.Errorf("unsupported node type %q: expected zicnode", cm.BaseConfig.NodeType)
	}
	switch cm.Protocol {
	case "vmess", "trojan", "hysteria2", "tuic", "anytls", "vless":
		node.Type = cm.Protocol
		node.Security = cm.Tls
	case "shadowsocks":
		node.Type = cm.Protocol
		node.Security = 0
	default:
		return nil, fmt.Errorf("unsupport protocol: %s", cm.Protocol)
	}
	node.Tag = fmt.Sprintf("[%s]-%s:%d", c.APIHost, node.Type, node.Id)
	certMode := strings.TrimSpace(cm.TlsSettings.CertMode)
	certDomain := strings.TrimSpace(cm.TlsSettings.PrimaryServerName())
	if certDomain == "" {
		certDomain = strings.TrimSpace(cm.Host)
	}
	cf := cm.TlsSettings.CertFile
	kf := cm.TlsSettings.KeyFile
	if cf == "" {
		cf = filepath.Join("/etc/zicnode/", "node-"+strconv.Itoa(c.NodeId)+".cer")
	}
	if kf == "" {
		kf = filepath.Join("/etc/zicnode/", "node-"+strconv.Itoa(c.NodeId)+".key")
	}
	cm.CertInfo = &CertInfo{
		CertMode:         certMode,
		CertFile:         cf,
		KeyFile:          kf,
		Email:            "node@zicboard.local",
		CertDomain:       certDomain,
		DNSEnv:           make(map[string]string),
		Provider:         cm.TlsSettings.Provider,
		SelfFallback:     cm.TlsSettings.SelfFallback,
		RejectUnknownSni: cm.TlsSettings.RejectUnknownSni == "1",
	}
	if cm.TlsSettings.DNSEnv != "" {
		envs := strings.FieldsFunc(cm.TlsSettings.DNSEnv, func(r rune) bool {
			return r == ',' || r == '\n' || r == '\r'
		})
		for _, env := range envs {
			kv := strings.SplitN(env, "=", 2)
			if len(kv) == 2 {
				key := strings.TrimSpace(kv[0])
				value := strings.TrimSpace(kv[1])
				if key != "" {
					cm.CertInfo.DNSEnv[key] = value
				}
			}
		}
	}

	// set interval
	node.PushInterval = intervalToTime(cm.BaseConfig.PushInterval)
	node.PullInterval = intervalToTime(cm.BaseConfig.PullInterval)

	node.Common = cm

	return node, nil
}

func intervalToTime(i interface{}) time.Duration {
	return time.Duration(flexibleIntFromAny(i)) * time.Second
}

func (t TlsSettings) EffectiveServerNames() []string {
	if len(t.ServerNames) > 0 {
		return t.ServerNames
	}
	if t.ServerName == "" {
		return nil
	}
	return []string{t.ServerName}
}

func (t TlsSettings) EffectiveShortIds() []string {
	if len(t.ShortIds) > 0 {
		return t.ShortIds
	}
	if t.ShortId == "" {
		return nil
	}
	return []string{t.ShortId}
}

func (t TlsSettings) PrimaryServerName() string {
	serverNames := t.EffectiveServerNames()
	if len(serverNames) == 0 {
		return ""
	}
	return serverNames[0]
}
