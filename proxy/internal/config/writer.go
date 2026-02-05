package config

import (
	"errors"
	"os"
	"sync"

	"github.com/sunshow/agentgear/proxy/internal/tagging"
	"github.com/sunshow/agentgear/proxy/internal/transformer"
	"gopkg.in/yaml.v3"
)

type ConfigWriter struct {
	configPath string
	updateCh   chan configUpdate
	mu         sync.RWMutex
	current    *Config
	done       chan struct{}
}

type updateType string

const (
	updateTypeGatewayAdd       updateType = "gateway_add"
	updateTypeGatewayUpdate    updateType = "gateway_update"
	updateTypeGatewayDelete    updateType = "gateway_delete"
	updateTypeTaggingUpdate    updateType = "tagging_update"
	updateTypeDefAdd           updateType = "def_add"
	updateTypeDefUpdate        updateType = "def_update"
	updateTypeDefDelete        updateType = "def_delete"
	updateTypeMappingAdd       updateType = "mapping_add"
	updateTypeMappingUpdate    updateType = "mapping_update"
	updateTypeMappingDelete    updateType = "mapping_delete"
)

type configUpdate struct {
	updateType updateType
	data       interface{}
	done       chan error
}

type gatewayUpdateData struct {
	name    string
	gateway GatewayConfig
}

type defUpdateData struct {
	name string
	def  transformer.TransformerDef
}

type mappingUpdateData struct {
	name    string
	mapping transformer.MappingRule
}

func NewConfigWriter(path string, cfg *Config) *ConfigWriter {
	w := &ConfigWriter{
		configPath: path,
		updateCh:   make(chan configUpdate, 100),
		current:    cfg,
		done:       make(chan struct{}),
	}
	go w.processUpdates()
	return w
}

func (w *ConfigWriter) Close() {
	close(w.updateCh)
	<-w.done
}

func (w *ConfigWriter) processUpdates() {
	defer close(w.done)
	for update := range w.updateCh {
		var err error
		switch update.updateType {
		case updateTypeGatewayAdd:
			gw := update.data.(GatewayConfig)
			err = w.addGateway(gw)
		case updateTypeGatewayUpdate:
			data := update.data.(gatewayUpdateData)
			err = w.updateGateway(data.name, data.gateway)
		case updateTypeGatewayDelete:
			name := update.data.(string)
			err = w.deleteGateway(name)
		case updateTypeTaggingUpdate:
			rules := update.data.([]tagging.Rule)
			err = w.updateTaggingRules(rules)
		case updateTypeDefAdd:
			def := update.data.(transformer.TransformerDef)
			err = w.addDefinition(def)
		case updateTypeDefUpdate:
			data := update.data.(defUpdateData)
			err = w.updateDefinition(data.name, data.def)
		case updateTypeDefDelete:
			name := update.data.(string)
			err = w.deleteDefinition(name)
		case updateTypeMappingAdd:
			mapping := update.data.(transformer.MappingRule)
			err = w.addMapping(mapping)
		case updateTypeMappingUpdate:
			data := update.data.(mappingUpdateData)
			err = w.updateMapping(data.name, data.mapping)
		case updateTypeMappingDelete:
			name := update.data.(string)
			err = w.deleteMapping(name)
		}
		if err == nil {
			err = w.saveToFile()
		}
		update.done <- err
	}
}

func (w *ConfigWriter) addGateway(gw GatewayConfig) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	for _, existing := range w.current.Gateways {
		if existing.Name == gw.Name {
			return errors.New("gateway with this name already exists")
		}
		if existing.Path == gw.Path {
			return errors.New("gateway with this path already exists")
		}
	}
	w.current.Gateways = append(w.current.Gateways, gw)
	return nil
}

func (w *ConfigWriter) updateGateway(name string, gw GatewayConfig) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	for i, existing := range w.current.Gateways {
		if existing.Name == name {
			if gw.Name != name {
				for _, other := range w.current.Gateways {
					if other.Name == gw.Name {
						return errors.New("gateway with this name already exists")
					}
				}
			}
			if gw.Path != existing.Path {
				for _, other := range w.current.Gateways {
					if other.Name != name && other.Path == gw.Path {
						return errors.New("gateway with this path already exists")
					}
				}
			}
			w.current.Gateways[i] = gw
			return nil
		}
	}
	return errors.New("gateway not found")
}

func (w *ConfigWriter) deleteGateway(name string) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	for i, existing := range w.current.Gateways {
		if existing.Name == name {
			w.current.Gateways = append(w.current.Gateways[:i], w.current.Gateways[i+1:]...)
			return nil
		}
	}
	return errors.New("gateway not found")
}

func (w *ConfigWriter) saveToFile() error {
	w.mu.RLock()
	defer w.mu.RUnlock()

	data, err := yaml.Marshal(w.current)
	if err != nil {
		return err
	}
	return os.WriteFile(w.configPath, data, 0644)
}

func (w *ConfigWriter) SaveToFile() error {
	return w.saveToFile()
}

func (w *ConfigWriter) AddGateway(gw GatewayConfig) error {
	done := make(chan error, 1)
	w.updateCh <- configUpdate{updateType: updateTypeGatewayAdd, data: gw, done: done}
	return <-done
}

func (w *ConfigWriter) UpdateGateway(name string, gw GatewayConfig) error {
	done := make(chan error, 1)
	w.updateCh <- configUpdate{
		updateType: updateTypeGatewayUpdate,
		data:       gatewayUpdateData{name: name, gateway: gw},
		done:       done,
	}
	return <-done
}

func (w *ConfigWriter) DeleteGateway(name string) error {
	done := make(chan error, 1)
	w.updateCh <- configUpdate{updateType: updateTypeGatewayDelete, data: name, done: done}
	return <-done
}

func (w *ConfigWriter) GetGateways() []GatewayConfig {
	w.mu.RLock()
	defer w.mu.RUnlock()
	result := make([]GatewayConfig, len(w.current.Gateways))
	copy(result, w.current.Gateways)
	return result
}

func (w *ConfigWriter) updateTaggingRules(rules []tagging.Rule) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.current.Tagging.Rules = rules
	return nil
}

func (w *ConfigWriter) UpdateTaggingRules(rules []tagging.Rule) error {
	done := make(chan error, 1)
	w.updateCh <- configUpdate{
		updateType: updateTypeTaggingUpdate,
		data:       rules,
		done:       done,
	}
	return <-done
}

func (w *ConfigWriter) GetTaggingRules() []tagging.Rule {
	w.mu.RLock()
	defer w.mu.RUnlock()
	result := make([]tagging.Rule, len(w.current.Tagging.Rules))
	copy(result, w.current.Tagging.Rules)
	return result
}

// Transformer Definition methods

func (w *ConfigWriter) addDefinition(def transformer.TransformerDef) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	for _, existing := range w.current.Transformers.Definitions {
		if existing.Name == def.Name {
			return errors.New("transformer definition already exists")
		}
	}
	w.current.Transformers.Definitions = append(w.current.Transformers.Definitions, def)
	return nil
}

func (w *ConfigWriter) updateDefinition(name string, def transformer.TransformerDef) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	for i, existing := range w.current.Transformers.Definitions {
		if existing.Name == name {
			if existing.Builtin {
				return errors.New("builtin definition cannot be modified")
			}
			w.current.Transformers.Definitions[i] = def
			return nil
		}
	}
	return errors.New("transformer definition not found")
}

func (w *ConfigWriter) deleteDefinition(name string) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	for i, existing := range w.current.Transformers.Definitions {
		if existing.Name == name {
			if existing.Builtin {
				return errors.New("builtin definition cannot be deleted")
			}
			w.current.Transformers.Definitions = append(
				w.current.Transformers.Definitions[:i],
				w.current.Transformers.Definitions[i+1:]...,
			)
			return nil
		}
	}
	return errors.New("transformer definition not found")
}

func (w *ConfigWriter) AddDefinition(def transformer.TransformerDef) error {
	done := make(chan error, 1)
	w.updateCh <- configUpdate{updateType: updateTypeDefAdd, data: def, done: done}
	return <-done
}

func (w *ConfigWriter) UpdateDefinition(name string, def transformer.TransformerDef) error {
	done := make(chan error, 1)
	w.updateCh <- configUpdate{
		updateType: updateTypeDefUpdate,
		data:       defUpdateData{name: name, def: def},
		done:       done,
	}
	return <-done
}

func (w *ConfigWriter) DeleteDefinition(name string) error {
	done := make(chan error, 1)
	w.updateCh <- configUpdate{updateType: updateTypeDefDelete, data: name, done: done}
	return <-done
}

func (w *ConfigWriter) GetDefinitions() []transformer.TransformerDef {
	w.mu.RLock()
	defer w.mu.RUnlock()
	result := make([]transformer.TransformerDef, len(w.current.Transformers.Definitions))
	copy(result, w.current.Transformers.Definitions)
	return result
}

// Mapping Rule methods

func (w *ConfigWriter) addMapping(mapping transformer.MappingRule) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	for _, existing := range w.current.Transformers.Mappings {
		if existing.Name == mapping.Name {
			return errors.New("mapping rule already exists")
		}
	}
	w.current.Transformers.Mappings = append(w.current.Transformers.Mappings, mapping)
	return nil
}

func (w *ConfigWriter) updateMapping(name string, mapping transformer.MappingRule) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	for i, existing := range w.current.Transformers.Mappings {
		if existing.Name == name {
			if existing.Builtin {
				return errors.New("builtin mapping cannot be modified")
			}
			w.current.Transformers.Mappings[i] = mapping
			return nil
		}
	}
	return errors.New("mapping rule not found")
}

func (w *ConfigWriter) deleteMapping(name string) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	for i, existing := range w.current.Transformers.Mappings {
		if existing.Name == name {
			if existing.Builtin {
				return errors.New("builtin mapping cannot be deleted")
			}
			w.current.Transformers.Mappings = append(
				w.current.Transformers.Mappings[:i],
				w.current.Transformers.Mappings[i+1:]...,
			)
			return nil
		}
	}
	return errors.New("mapping rule not found")
}

func (w *ConfigWriter) AddMapping(mapping transformer.MappingRule) error {
	done := make(chan error, 1)
	w.updateCh <- configUpdate{updateType: updateTypeMappingAdd, data: mapping, done: done}
	return <-done
}

func (w *ConfigWriter) UpdateMapping(name string, mapping transformer.MappingRule) error {
	done := make(chan error, 1)
	w.updateCh <- configUpdate{
		updateType: updateTypeMappingUpdate,
		data:       mappingUpdateData{name: name, mapping: mapping},
		done:       done,
	}
	return <-done
}

func (w *ConfigWriter) DeleteMapping(name string) error {
	done := make(chan error, 1)
	w.updateCh <- configUpdate{updateType: updateTypeMappingDelete, data: name, done: done}
	return <-done
}

func (w *ConfigWriter) GetMappings() []transformer.MappingRule {
	w.mu.RLock()
	defer w.mu.RUnlock()
	result := make([]transformer.MappingRule, len(w.current.Transformers.Mappings))
	copy(result, w.current.Transformers.Mappings)
	return result
}
