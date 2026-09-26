// Package tools implements the full Bordo MCP tool surface.
package tools

// NewBordoToolRegistry creates and registers all Bordo MCP tools.
func NewBordoToolRegistry(cp *CPClient) *ToolRegistry {
	r := NewToolRegistry()

	r.Register(PingTool())

	// Template catalog
	r.Register(templateListTool(cp))

	// Project tools
	r.Register(projectCreateTool(cp))
	r.Register(projectListTool(cp))
	r.Register(projectGetTool(cp))
	r.Register(projectDeleteTool(cp))

	// Build tools
	r.Register(buildListTool(cp))
	r.Register(buildTriggerTool(cp))
	r.Register(buildStatusTool(cp))
	r.Register(buildLogsTool(cp))

	// Deploy / release tools
	r.Register(deployListTool(cp))
	r.Register(deployTool(cp))
	r.Register(deployStatusTool(cp))
	r.Register(rollbackTool(cp))

	// Observe tools (stub until BRD-030)
	r.Register(metricsQueryTool(cp))
	r.Register(logsQueryTool(cp))

	// Region / fleet tools
	r.Register(regionListTool(cp))
	r.Register(regionAddTool(cp))

	// GitHub integration
	r.Register(githubCreateRepoTool(cp))

	return r
}
