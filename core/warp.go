package core

import (
	"bytes"
	"crypto/ecdh"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	panel "github.com/ZicBoard/ZicNode/api/zicboard"
	"github.com/xtls/xray-core/core"
	coreConf "github.com/xtls/xray-core/infra/conf"
)

const (
	warpDefaultEndpoint = "engage.cloudflareclient.com:2408"
	warpDefaultMTU      = 1280
	warpAPITimeout      = 15 * time.Second
)

var warpDefaultAddresses = []string{"172.16.0.2/32", "2606:4700:110:8765::2/128"}

type warpRuntimeConfig struct {
	PrivateKey     string
	PublicKey      string
	PeerPublicKey  string
	Endpoint       string
	Addresses      []string
	Reserved       []byte
	MTU            int
	DomainStrategy string
	Source         string
}

type warpSidecar struct {
	Version       int       `json:"version"`
	NodeID        int       `json:"node_id"`
	NodeTag       string    `json:"node_tag"`
	PrivateKey    string    `json:"private_key"`
	PublicKey     string    `json:"public_key"`
	PeerPublicKey string    `json:"peer_public_key"`
	Endpoint      string    `json:"endpoint"`
	Addresses     []string  `json:"addresses"`
	Reserved      []int     `json:"reserved"`
	DeviceID      string    `json:"device_id,omitempty"`
	Token         string    `json:"token,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

func warpEnabled(info *panel.NodeInfo) bool {
	return info != nil && info.Common != nil && info.Common.WarpSettings.Enable
}

func warpFailPolicy(info *panel.NodeInfo) string {
	if info == nil || info.Common == nil || strings.EqualFold(info.Common.WarpSettings.FailPolicy, "block") {
		return "block"
	}
	return "direct"
}

func warpOutboundTag(info *panel.NodeInfo) string {
	if info == nil || strings.TrimSpace(info.Tag) == "" {
		return "warp-zicnode"
	}
	return "warp-" + info.Tag
}

func buildWarpOutbound(info *panel.NodeInfo) (*core.OutboundHandlerConfig, string, error) {
	if !warpEnabled(info) {
		return nil, "", nil
	}

	runtimeConfig, err := resolveWarpRuntimeConfig(info)
	if err != nil {
		return nil, warpOutboundTag(info), err
	}

	settings := map[string]interface{}{
		"secretKey":      runtimeConfig.PrivateKey,
		"address":        runtimeConfig.Addresses,
		"peers":          []map[string]interface{}{{"publicKey": runtimeConfig.PeerPublicKey, "endpoint": runtimeConfig.Endpoint, "allowedIPs": []string{"0.0.0.0/0", "::/0"}}},
		"mtu":            runtimeConfig.MTU,
		"domainStrategy": runtimeConfig.DomainStrategy,
		"noKernelTun":    true,
	}
	if len(runtimeConfig.Reserved) > 0 {
		settings["reserved"] = byteSliceToInts(runtimeConfig.Reserved)
	}

	rawSettings, err := json.Marshal(settings)
	if err != nil {
		return nil, warpOutboundTag(info), fmt.Errorf("marshal WARP settings: %w", err)
	}
	outbound := &coreConf.OutboundDetourConfig{
		Protocol: "wireguard",
		Tag:      warpOutboundTag(info),
		Settings: (*json.RawMessage)(&rawSettings),
	}
	built, err := outbound.Build()
	if err != nil {
		return nil, outbound.Tag, fmt.Errorf("build WARP outbound: %w", err)
	}
	return built, outbound.Tag, nil
}

func resolveWarpRuntimeConfig(info *panel.NodeInfo) (*warpRuntimeConfig, error) {
	settings := normalizeWarpSettings(info.Common.WarpSettings)
	if hasManualWarpIdentity(settings) {
		return runtimeFromWarpSettings(settings, "manual"), nil
	}
	if settings.Mode == "manual" {
		return nil, fmt.Errorf("manual WARP mode requires private_key and peer_public_key")
	}

	if sidecarConfig, err := loadWarpSidecar(info, settings); err == nil {
		return sidecarConfig, nil
	}

	registered, err := registerWarpIdentity(settings)
	if err != nil {
		return nil, err
	}
	if err := saveWarpSidecar(info, registered); err != nil {
		return nil, err
	}
	return registered, nil
}

func normalizeWarpSettings(settings panel.WarpSettings) panel.WarpSettings {
	settings.Mode = strings.ToLower(strings.TrimSpace(settings.Mode))
	if settings.Mode != "manual" {
		settings.Mode = "auto"
	}
	settings.FailPolicy = strings.ToLower(strings.TrimSpace(settings.FailPolicy))
	if settings.FailPolicy != "block" {
		settings.FailPolicy = "direct"
	}
	if settings.MTU <= 0 {
		settings.MTU = warpDefaultMTU
	}
	if !validWarpDomainStrategy(settings.DomainStrategy) {
		settings.DomainStrategy = "ForceIPv4v6"
	}
	settings.PrivateKey = strings.TrimSpace(settings.PrivateKey)
	settings.PeerPublicKey = strings.TrimSpace(settings.PeerPublicKey)
	settings.Endpoint = strings.TrimSpace(settings.Endpoint)
	if settings.Endpoint == "" {
		settings.Endpoint = warpDefaultEndpoint
	}
	settings.Addresses = cleanStringList(settings.Addresses)
	if len(settings.Addresses) == 0 {
		settings.Addresses = append([]string(nil), warpDefaultAddresses...)
	}
	return settings
}

func hasManualWarpIdentity(settings panel.WarpSettings) bool {
	return strings.TrimSpace(settings.PrivateKey) != "" && strings.TrimSpace(settings.PeerPublicKey) != ""
}

func runtimeFromWarpSettings(settings panel.WarpSettings, source string) *warpRuntimeConfig {
	return &warpRuntimeConfig{
		PrivateKey:     settings.PrivateKey,
		PeerPublicKey:  settings.PeerPublicKey,
		Endpoint:       settings.Endpoint,
		Addresses:      append([]string(nil), settings.Addresses...),
		Reserved:       append([]byte(nil), settings.Reserved...),
		MTU:            settings.MTU,
		DomainStrategy: settings.DomainStrategy,
		Source:         source,
	}
}

func registerWarpIdentity(settings panel.WarpSettings) (*warpRuntimeConfig, error) {
	privateKey, publicKey, err := generateWireGuardKeyPair()
	if err != nil {
		return nil, err
	}

	body, err := json.Marshal(map[string]interface{}{
		"key":        publicKey,
		"install_id": "",
		"fcm_token":  "",
		"tos":        time.Now().Format(time.RFC3339),
		"type":       "Android",
		"locale":     "en_US",
	})
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest(http.MethodPost, "https://api.cloudflareclient.com/v0a2158/reg", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "okhttp/3.12.1")
	req.Header.Set("CF-Client-Version", "a-6.10-2158")

	client := &http.Client{Timeout: warpAPITimeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("register WARP identity: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("read WARP register response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("register WARP identity failed: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(respBody, &raw); err != nil {
		return nil, fmt.Errorf("decode WARP register response: %w", err)
	}
	config, err := runtimeFromWarpRegisterResponse(raw, settings, privateKey, publicKey)
	if err != nil {
		return nil, err
	}
	config.Source = "auto"
	return config, nil
}

func generateWireGuardKeyPair() (privateKey string, publicKey string, err error) {
	privateBytes := make([]byte, 32)
	if _, err := rand.Read(privateBytes); err != nil {
		return "", "", fmt.Errorf("generate WARP private key: %w", err)
	}
	privateBytes[0] &= 248
	privateBytes[31] = (privateBytes[31] & 127) | 64

	key, err := ecdh.X25519().NewPrivateKey(privateBytes)
	if err != nil {
		return "", "", fmt.Errorf("derive WARP public key: %w", err)
	}
	return base64.StdEncoding.EncodeToString(privateBytes), base64.StdEncoding.EncodeToString(key.PublicKey().Bytes()), nil
}

func runtimeFromWarpRegisterResponse(raw map[string]interface{}, settings panel.WarpSettings, privateKey, publicKey string) (*warpRuntimeConfig, error) {
	configMap := mapValue(raw, "config")
	peerMap := firstMap(sliceValue(configMap, "peers"))
	peerPublicKey := firstNonEmpty(settings.PeerPublicKey, stringValue(peerMap, "public_key"), stringValue(peerMap, "publicKey"))
	if peerPublicKey == "" {
		return nil, fmt.Errorf("WARP register response missing peer public key")
	}

	endpoint := firstNonEmpty(nonDefaultWarpEndpoint(settings.Endpoint), endpointFromPeer(peerMap), warpDefaultEndpoint)
	addresses := addressesFromWarpResponse(configMap)
	if len(addresses) == 0 || !isDefaultWarpAddresses(settings.Addresses) {
		addresses = append([]string(nil), settings.Addresses...)
	}
	reserved := append([]byte(nil), settings.Reserved...)
	if len(reserved) == 0 {
		reserved = reservedFromWarpResponse(raw, configMap)
	}

	return &warpRuntimeConfig{
		PrivateKey:     privateKey,
		PublicKey:      publicKey,
		PeerPublicKey:  peerPublicKey,
		Endpoint:       endpoint,
		Addresses:      addresses,
		Reserved:       reserved,
		MTU:            settings.MTU,
		DomainStrategy: settings.DomainStrategy,
	}, nil
}

func loadWarpSidecar(info *panel.NodeInfo, settings panel.WarpSettings) (*warpRuntimeConfig, error) {
	path := warpSidecarPath(info)
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var sidecar warpSidecar
	if err := json.Unmarshal(raw, &sidecar); err != nil {
		return nil, err
	}
	if sidecar.PrivateKey == "" || sidecar.PeerPublicKey == "" {
		return nil, fmt.Errorf("WARP sidecar missing keys")
	}

	endpoint := firstNonEmpty(nonDefaultWarpEndpoint(settings.Endpoint), sidecar.Endpoint, warpDefaultEndpoint)
	addresses := cleanStringList(sidecar.Addresses)
	if len(addresses) == 0 || !isDefaultWarpAddresses(settings.Addresses) {
		addresses = append([]string(nil), settings.Addresses...)
	}
	reserved := intsToByteSlice(sidecar.Reserved)
	if len(settings.Reserved) > 0 {
		reserved = append([]byte(nil), settings.Reserved...)
	}

	return &warpRuntimeConfig{
		PrivateKey:     sidecar.PrivateKey,
		PublicKey:      sidecar.PublicKey,
		PeerPublicKey:  firstNonEmpty(settings.PeerPublicKey, sidecar.PeerPublicKey),
		Endpoint:       endpoint,
		Addresses:      addresses,
		Reserved:       reserved,
		MTU:            settings.MTU,
		DomainStrategy: settings.DomainStrategy,
		Source:         "sidecar",
	}, nil
}

func saveWarpSidecar(info *panel.NodeInfo, runtimeConfig *warpRuntimeConfig) error {
	path := warpSidecarPath(info)
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return fmt.Errorf("create WARP sidecar directory: %w", err)
	}

	now := time.Now().UTC()
	sidecar := warpSidecar{
		Version:       1,
		NodeID:        info.Id,
		NodeTag:       info.Tag,
		PrivateKey:    runtimeConfig.PrivateKey,
		PublicKey:     runtimeConfig.PublicKey,
		PeerPublicKey: runtimeConfig.PeerPublicKey,
		Endpoint:      runtimeConfig.Endpoint,
		Addresses:     runtimeConfig.Addresses,
		Reserved:      byteSliceToInts(runtimeConfig.Reserved),
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	raw, err := json.MarshalIndent(sidecar, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal WARP sidecar: %w", err)
	}
	if err := os.WriteFile(path, raw, 0600); err != nil {
		return fmt.Errorf("write WARP sidecar: %w", err)
	}
	return nil
}

func warpSidecarPath(info *panel.NodeInfo) string {
	dir := strings.TrimSpace(os.Getenv("ZICNODE_WARP_DIR"))
	if dir == "" {
		dir = filepath.Join(string(os.PathSeparator), "etc", "zicnode", "warp")
	}
	hashInput := warpPanelIdentity(info)
	hash := sha256.Sum256([]byte(hashInput))
	nodeID := 0
	if info != nil {
		nodeID = info.Id
	}
	return filepath.Join(dir, hex.EncodeToString(hash[:])[:12]+"-zicnode-"+strconv.Itoa(nodeID)+".json")
}

func warpPanelIdentity(info *panel.NodeInfo) string {
	if info == nil {
		return "zicnode"
	}
	tag := strings.TrimSpace(info.Tag)
	if strings.HasPrefix(tag, "[") {
		if end := strings.Index(tag, "]"); end > 1 {
			return tag[1:end]
		}
	}
	if tag != "" {
		return tag
	}
	return "zicnode"
}

func validWarpDomainStrategy(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "forceip", "forceipv4", "forceipv6", "forceipv4v6", "forceipv6v4":
		return true
	default:
		return false
	}
}

func cleanStringList(values []string) []string {
	cleaned := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			cleaned = append(cleaned, value)
		}
	}
	return cleaned
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func nonDefaultWarpEndpoint(endpoint string) string {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" || strings.EqualFold(endpoint, warpDefaultEndpoint) {
		return ""
	}
	return endpoint
}

func isDefaultWarpAddresses(addresses []string) bool {
	addresses = cleanStringList(addresses)
	if len(addresses) != len(warpDefaultAddresses) {
		return false
	}
	for i, address := range addresses {
		if address != warpDefaultAddresses[i] {
			return false
		}
	}
	return true
}

func mapValue(raw map[string]interface{}, key string) map[string]interface{} {
	if raw == nil {
		return nil
	}
	value, _ := raw[key].(map[string]interface{})
	return value
}

func sliceValue(raw map[string]interface{}, key string) []interface{} {
	if raw == nil {
		return nil
	}
	value, _ := raw[key].([]interface{})
	return value
}

func firstMap(values []interface{}) map[string]interface{} {
	if len(values) == 0 {
		return nil
	}
	value, _ := values[0].(map[string]interface{})
	return value
}

func stringValue(raw map[string]interface{}, key string) string {
	if raw == nil {
		return ""
	}
	switch value := raw[key].(type) {
	case string:
		return strings.TrimSpace(value)
	case float64:
		return strconv.FormatFloat(value, 'f', -1, 64)
	case bool:
		if value {
			return "true"
		}
		return "false"
	default:
		return ""
	}
}

func endpointFromPeer(peer map[string]interface{}) string {
	if peer == nil {
		return ""
	}
	if value := stringValue(peer, "endpoint"); value != "" {
		return value
	}
	endpointMap := mapValue(peer, "endpoint")
	host := firstNonEmpty(stringValue(endpointMap, "host"), stringValue(endpointMap, "v4"), stringValue(endpointMap, "v6"))
	if host == "" {
		return ""
	}
	port := "2408"
	ports := sliceValue(endpointMap, "ports")
	if len(ports) > 0 {
		if p := numberString(ports[0]); p != "" {
			port = p
		}
	}
	return host + ":" + port
}

func addressesFromWarpResponse(configMap map[string]interface{}) []string {
	interfaceMap := mapValue(configMap, "interface")
	addressesMap := mapValue(interfaceMap, "addresses")
	addresses := make([]string, 0, 2)
	if v4 := stringValue(addressesMap, "v4"); v4 != "" {
		addresses = append(addresses, ensureCIDR(v4, "/32"))
	}
	if v6 := stringValue(addressesMap, "v6"); v6 != "" {
		addresses = append(addresses, ensureCIDR(v6, "/128"))
	}
	if len(addresses) == 0 {
		for _, item := range sliceValue(interfaceMap, "addresses") {
			if address := ensureCIDRFromAny(item); address != "" {
				addresses = append(addresses, address)
			}
		}
	}
	return cleanStringList(addresses)
}

func ensureCIDRFromAny(value interface{}) string {
	address := ""
	switch typed := value.(type) {
	case string:
		address = strings.TrimSpace(typed)
	case map[string]interface{}:
		address = firstNonEmpty(stringValue(typed, "v4"), stringValue(typed, "v6"), stringValue(typed, "address"))
	}
	if address == "" || strings.Contains(address, "/") {
		return address
	}
	if strings.Contains(address, ":") {
		return address + "/128"
	}
	return address + "/32"
}

func ensureCIDR(address, suffix string) string {
	address = strings.TrimSpace(address)
	if address == "" || strings.Contains(address, "/") {
		return address
	}
	return address + suffix
}

func reservedFromWarpResponse(raw, configMap map[string]interface{}) []byte {
	if reserved := reservedArray(raw["reserved"]); len(reserved) > 0 {
		return reserved
	}
	if reserved := reservedArray(configMap["reserved"]); len(reserved) > 0 {
		return reserved
	}
	clientID := firstNonEmpty(stringValue(configMap, "client_id"), stringValue(raw, "client_id"), stringValue(configMap, "clientId"), stringValue(raw, "clientId"))
	if clientID == "" {
		return nil
	}
	for _, encoding := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding, base64.URLEncoding, base64.RawURLEncoding} {
		decoded, err := encoding.DecodeString(clientID)
		if err == nil && len(decoded) >= 3 {
			return decoded[:3]
		}
	}
	return nil
}

func reservedArray(value interface{}) []byte {
	items, ok := value.([]interface{})
	if !ok || len(items) == 0 {
		return nil
	}
	reserved := make([]byte, 0, len(items))
	for _, item := range items {
		value, ok := intFromAny(item)
		if !ok || value < 0 || value > 255 {
			return nil
		}
		reserved = append(reserved, byte(value))
	}
	return reserved
}

func intFromAny(value interface{}) (int, bool) {
	switch typed := value.(type) {
	case float64:
		return int(typed), true
	case int:
		return typed, true
	case string:
		parsed, err := strconv.Atoi(strings.TrimSpace(typed))
		return parsed, err == nil
	default:
		return 0, false
	}
}

func numberString(value interface{}) string {
	switch typed := value.(type) {
	case float64:
		return strconv.Itoa(int(typed))
	case int:
		return strconv.Itoa(typed)
	case string:
		return strings.TrimSpace(typed)
	default:
		return ""
	}
}

func byteSliceToInts(values []byte) []int {
	if len(values) == 0 {
		return nil
	}
	items := make([]int, 0, len(values))
	for _, value := range values {
		items = append(items, int(value))
	}
	return items
}

func intsToByteSlice(values []int) []byte {
	if len(values) == 0 {
		return nil
	}
	items := make([]byte, 0, len(values))
	for _, value := range values {
		if value >= 0 && value <= 255 {
			items = append(items, byte(value))
		}
	}
	return items
}
