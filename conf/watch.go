package conf

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/fsnotify/fsnotify"
)

func fileHash(filePath string) (string, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func (p *Conf) Watch(filePath string, reload func()) error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("new watcher error: %s", err)
	}
	// Track the current content hash so editor/atomic writes that do not change
	// the actual config do not trigger a reload (which disconnects users).
	lastHash, _ := fileHash(filePath)
	go func() {
		var pre time.Time
		defer watcher.Close()
		for {
			select {
			case e := <-watcher.Events:
				if e.Has(fsnotify.Chmod) {
					continue
				}
				if pre.Add(10 * time.Second).After(time.Now()) {
					continue
				}
				pre = time.Now()
				go func() {
					time.Sleep(5 * time.Second)
					newHash, err := fileHash(filePath)
					if err != nil {
						log.Printf("read config for change detection error: %s", err)
						return
					}
					if newHash == lastHash {
						log.Println("config file event ignored: content unchanged")
						return
					}
					log.Println("config file changed, reloading...")
					*p = *New()
					if err := p.LoadFromPath(filePath); err != nil {
						log.Printf("reload config error: %s", err)
						return
					}
					lastHash = newHash
					reload()
					log.Println("reload config success")
				}()
			case err := <-watcher.Errors:
				if err != nil {
					log.Printf("File watcher error: %s", err)
				}
			}
		}
	}()
	err = watcher.Add(filePath)
	if err != nil {
		return fmt.Errorf("watch file error: %s", err)
	}
	return nil
}
