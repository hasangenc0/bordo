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
	r.Register(projectEnableBuildsTool(cp))
	r.Register(projectCreateRepoTool(cp))
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
	r.Register(deployGroupStatusTool(cp))
	r.Register(deployLogsTool(cp))
	r.Register(rollbackTool(cp))
	r.Register(deployWaitTool(cp))
	r.Register(appEndpointsTool(cp))
	r.Register(appProbeTool(cp))

	// Observe tools
	r.Register(metricsQueryTool(cp))
	r.Register(logsQueryTool(cp))

	// Code tools (workspace git: checkout, edit, diff, commit, PR)
	for _, t := range NewCodeTools(cp, "").Tools() {
		r.Register(t)
	}

	// Region / fleet tools
	r.Register(regionListTool(cp))
	r.Register(regionAddTool(cp))
	r.Register(regionSetHostTool(cp))
	r.Register(regionHealthTool(cp))
	r.Register(fleetOverviewTool(cp))

	return r
}
