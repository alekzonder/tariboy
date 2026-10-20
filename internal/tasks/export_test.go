package tasks

import "database/sql"

// The external tests in this directory (package tasks_test) import taskgoal,
// which imports tasks; these hooks give them the internal fixtures.

// RequestFixture is requestFixture: DEV bound to requestDefinition and one
// task held by dev-1 in "develop".
var RequestFixture = requestFixture

// ServiceDB is the service's database handle.
func ServiceDB(s *Service) *sql.DB { return s.db }

// SetAgent is setAgent: SQL assignments to one agent's registry row.
var SetAgent = setAgent

// AgentGoalTaskKey is agentGoalTaskKey: the agent's current Goal task key.
var AgentGoalTaskKey = agentGoalTaskKey
