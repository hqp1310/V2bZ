package reload

const (
	ReasonManualReload       Reason = "manual_reload"
	ReasonConfigFileChanged  Reason = "config_file_changed"
	ReasonNodeConfigChanged  Reason = "node_config_changed"
	ReasonTaskTimeout        Reason = "task_timeout"
	ReasonCertMetadataChange Reason = "cert_metadata_changed"
)

type Reason string

type Event struct {
	Reason   Reason
	NodeTag  string
	TaskName string
	Details  map[string]string
}

func (e Event) ReasonString() string {
	if e.Reason == "" {
		return string(ReasonManualReload)
	}
	return string(e.Reason)
}
