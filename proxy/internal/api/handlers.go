package api

import (
	"net/http"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sunshow/agentgear/proxy/internal/config"
	"github.com/sunshow/agentgear/proxy/internal/memory"
	"github.com/sunshow/agentgear/proxy/internal/tagging"
	"github.com/sunshow/agentgear/proxy/internal/transformer"
)

func (s *Server) healthCheck(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (s *Server) getStats(c *gin.Context) {
	stats := s.store.Stats()
	c.JSON(http.StatusOK, stats)
}

func (s *Server) listConnections(c *gin.Context) {
	limitStr := c.DefaultQuery("limit", "100")
	limit, _ := strconv.Atoi(limitStr)
	if limit <= 0 {
		limit = 100
	}

	tagsStr := c.Query("tags")
	status := c.Query("status")

	var connections []*memory.ConnectionInfo

	if tagsStr != "" {
		tags := strings.Split(tagsStr, ",")
		connections = s.store.FilterByTags(tags, limit)
	} else {
		connections = s.store.GetRecent(limit)
	}

	if status != "" {
		filtered := make([]*memory.ConnectionInfo, 0)
		for _, conn := range connections {
			if conn.Status == status {
				filtered = append(filtered, conn)
			}
		}
		connections = filtered
	}

	dtos := ToConnectionDTOList(connections)
	c.JSON(http.StatusOK, gin.H{
		"connections": dtos,
		"total":       len(dtos),
	})
}

func (s *Server) getConnection(c *gin.Context) {
	id := c.Param("id")
	conn := s.store.Get(id)
	if conn == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "connection not found"})
		return
	}
	dto := ToConnectionDTO(conn)
	c.JSON(http.StatusOK, dto)
}

func (s *Server) clearConnections(c *gin.Context) {
	s.store.Clear()
	c.JSON(http.StatusOK, gin.H{"status": "cleared"})
}

func (s *Server) listTags(c *gin.Context) {
	stats := s.store.Stats()
	tagCounts, _ := stats["by_tag"].(map[string]int)

	tags := make([]map[string]interface{}, 0)
	for tag, count := range tagCounts {
		tags = append(tags, map[string]interface{}{
			"name":  tag,
			"count": count,
		})
	}

	c.JSON(http.StatusOK, gin.H{"tags": tags})
}

func (s *Server) listTaggingRules(c *gin.Context) {
	rules := s.tagging.GetRules()
	c.JSON(http.StatusOK, gin.H{"rules": rules})
}

func (s *Server) updateTaggingRule(c *gin.Context) {
	name := c.Param("name")

	var rule tagging.Rule
	if err := c.ShouldBindJSON(&rule); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	rules := s.tagging.GetRules()
	found := false
	for i, r := range rules {
		if r.Name == name {
			rules[i] = rule
			found = true
			break
		}
	}

	if !found {
		rules = append(rules, rule)
	}

	s.tagging.UpdateRules(rules)

	// Sync to ConfigWriter (only save non-builtin rules)
	userRules := make([]tagging.Rule, 0)
	for _, r := range rules {
		if !r.Builtin {
			userRules = append(userRules, r)
		}
	}
	s.configWriter.UpdateTaggingRules(userRules)

	c.JSON(http.StatusOK, gin.H{"status": "updated"})
}

func (s *Server) deleteTaggingRule(c *gin.Context) {
	name := c.Param("name")

	rules := s.tagging.GetRules()
	newRules := make([]tagging.Rule, 0)
	found := false

	for _, r := range rules {
		if r.Name == name {
			if r.Builtin {
				c.JSON(http.StatusBadRequest, gin.H{"error": "cannot delete builtin rule"})
				return
			}
			found = true
			continue
		}
		if !r.Builtin {
			newRules = append(newRules, r)
		}
	}

	if !found {
		c.JSON(http.StatusNotFound, gin.H{"error": "rule not found"})
		return
	}

	s.tagging.UpdateRules(newRules)

	// Sync to ConfigWriter
	s.configWriter.UpdateTaggingRules(newRules)

	c.JSON(http.StatusOK, gin.H{"status": "deleted"})
}

func (s *Server) testTagging(c *gin.Context) {
	var req struct {
		Method  string            `json:"method"`
		Path    string            `json:"path"`
		Headers map[string]string `json:"headers"`
		Body    string            `json:"body"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	headers := make(http.Header)
	for k, v := range req.Headers {
		headers.Set(k, v)
	}

	ctx := &tagging.RequestContext{
		Method:  req.Method,
		Path:    req.Path,
		Headers: headers,
		Body:    []byte(req.Body),
	}

	result := s.tagging.TestMatch(ctx)
	c.JSON(http.StatusOK, result)
}

func (s *Server) listGateways(c *gin.Context) {
	gateways := s.configWriter.GetGateways()
	c.JSON(http.StatusOK, gin.H{"gateways": gateways})
}

func (s *Server) createGateway(c *gin.Context) {
	var gw config.GatewayConfig
	if err := c.ShouldBindJSON(&gw); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if gw.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name is required"})
		return
	}
	if gw.Path == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "path is required"})
		return
	}
	if gw.Upstream == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "upstream is required"})
		return
	}
	if gw.Timeout <= 0 {
		gw.Timeout = 300
	}

	if err := s.configWriter.AddGateway(gw); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "created"})
}

func (s *Server) updateGateway(c *gin.Context) {
	name := c.Param("name")

	var gw config.GatewayConfig
	if err := c.ShouldBindJSON(&gw); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if gw.Name == "" {
		gw.Name = name
	}
	if gw.Timeout <= 0 {
		gw.Timeout = 300
	}

	if err := s.configWriter.UpdateGateway(name, gw); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "updated"})
}

func (s *Server) deleteGateway(c *gin.Context) {
	name := c.Param("name")

	if err := s.configWriter.DeleteGateway(name); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "deleted"})
}

func (s *Server) restartProxy(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "restarting"})

	go func() {
		time.Sleep(100 * time.Millisecond)
		executable, err := os.Executable()
		if err != nil {
			return
		}
		syscall.Exec(executable, os.Args, os.Environ())
	}()
}

func (s *Server) saveConfig(c *gin.Context) {
	if err := s.configWriter.SaveToFile(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "saved"})
}

// Transformer Definitions handlers

func (s *Server) listDefinitions(c *gin.Context) {
	defs := s.transformer.GetDefinitions()
	c.JSON(http.StatusOK, gin.H{"definitions": defs})
}

func (s *Server) createDefinition(c *gin.Context) {
	var def transformer.TransformerDef
	if err := c.ShouldBindJSON(&def); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if def.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name is required"})
		return
	}
	if def.Direction != "request" && def.Direction != "response" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "direction must be 'request' or 'response'"})
		return
	}

	// Validate type-specific required fields
	defType := def.Type
	if defType == "" {
		defType = "tool" // Default type
	}

	switch defType {
	case "tool":
		if def.SourceTool == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "source_tool is required for tool type"})
			return
		}
		if def.TargetTool == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "target_tool is required for tool type"})
			return
		}
	case "message_inject":
		if def.InjectText == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "inject_text is required for message_inject type"})
			return
		}
		if def.Direction != "request" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "message_inject only supports request direction"})
			return
		}
	case "error_transform":
		if def.ErrorCode == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "error_code is required for error_transform type"})
			return
		}
		if def.ErrorMessage == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "error_message is required for error_transform type"})
			return
		}
		if def.Direction != "response" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "error_transform only supports response direction"})
			return
		}
	case "header_inject":
		if len(def.HeaderInjections) == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "header_injections is required for header_inject type"})
			return
		}
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid type, must be: tool, message_inject, error_transform, or header_inject"})
		return
	}

	if err := s.transformer.AddDefinition(def); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := s.configWriter.AddDefinition(def); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "created"})
}

func (s *Server) updateDefinition(c *gin.Context) {
	name := c.Param("name")

	var def transformer.TransformerDef
	if err := c.ShouldBindJSON(&def); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if def.Name == "" {
		def.Name = name
	}

	// Validate type-specific required fields
	defType := def.Type
	if defType == "" {
		defType = "tool" // Default type
	}

	switch defType {
	case "tool":
		if def.SourceTool == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "source_tool is required for tool type"})
			return
		}
		if def.TargetTool == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "target_tool is required for tool type"})
			return
		}
	case "message_inject":
		if def.InjectText == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "inject_text is required for message_inject type"})
			return
		}
		if def.Direction != "request" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "message_inject only supports request direction"})
			return
		}
	case "error_transform":
		if def.ErrorCode == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "error_code is required for error_transform type"})
			return
		}
		if def.ErrorMessage == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "error_message is required for error_transform type"})
			return
		}
		if def.Direction != "response" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "error_transform only supports response direction"})
			return
		}
	case "header_inject":
		if len(def.HeaderInjections) == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "header_injections is required for header_inject type"})
			return
		}
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid type, must be: tool, message_inject, error_transform, or header_inject"})
		return
	}

	if err := s.transformer.UpdateDefinition(name, def); err != nil {
		if err == transformer.ErrDefinitionNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		} else {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		}
		return
	}

	if err := s.configWriter.UpdateDefinition(name, def); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "updated"})
}

func (s *Server) deleteDefinition(c *gin.Context) {
	name := c.Param("name")

	if err := s.transformer.DeleteDefinition(name); err != nil {
		if err == transformer.ErrDefinitionNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		} else {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		}
		return
	}

	if err := s.configWriter.DeleteDefinition(name); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "deleted"})
}

// Mapping Rules handlers

func (s *Server) listMappings(c *gin.Context) {
	mappings := s.transformer.GetMappings()
	c.JSON(http.StatusOK, gin.H{"mappings": mappings})
}

func (s *Server) createMapping(c *gin.Context) {
	var mapping transformer.MappingRule
	if err := c.ShouldBindJSON(&mapping); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if mapping.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name is required"})
		return
	}
	if mapping.Transformer == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "transformer is required"})
		return
	}

	if err := s.transformer.AddMapping(mapping); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := s.configWriter.AddMapping(mapping); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "created"})
}

func (s *Server) updateMappingRule(c *gin.Context) {
	name := c.Param("name")

	var mapping transformer.MappingRule
	if err := c.ShouldBindJSON(&mapping); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if mapping.Name == "" {
		mapping.Name = name
	}

	if err := s.transformer.UpdateMapping(name, mapping); err != nil {
		if err == transformer.ErrMappingNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		} else {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		}
		return
	}

	if err := s.configWriter.UpdateMapping(name, mapping); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "updated"})
}

func (s *Server) deleteMappingRule(c *gin.Context) {
	name := c.Param("name")

	if err := s.transformer.DeleteMapping(name); err != nil {
		if err == transformer.ErrMappingNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		} else {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		}
		return
	}

	if err := s.configWriter.DeleteMapping(name); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "deleted"})
}
