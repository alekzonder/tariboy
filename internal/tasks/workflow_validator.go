package tasks

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

// WorkflowValidationError is a stable, machine-readable problem in a workflow
// definition. Path points at the offending JSON field when one is available.
type WorkflowValidationError struct {
	Code    string `json:"code"`
	Path    string `json:"path,omitempty"`
	Message string `json:"message"`
}

// ValidateWorkflow validates a workflow without reading or changing durable
// state. It returns every independently discoverable problem in source order.
func ValidateWorkflow(def WorkflowDefinition) []WorkflowValidationError {
	validator := workflowValidator{definition: normalizeWorkflowDefinition(def)}
	validator.validate()
	return validator.errors
}

// CanonicalWorkflowDefinition returns the exact normalized representation used
// for workflow validation and persistence. Declarative clients use it to
// distinguish harmless source formatting from a semantic change to an
// immutable published version.
func CanonicalWorkflowDefinition(def WorkflowDefinition) WorkflowDefinition {
	return normalizeWorkflowDefinition(def)
}

// normalizeWorkflowDefinition returns a detached canonical definition. The
// previous_outputs shortcut expands to the union of artifacts that a preceding
// status execution can produce, including prior passes through a cycle.
func normalizeWorkflowDefinition(def WorkflowDefinition) WorkflowDefinition {
	def.Name = strings.TrimSpace(def.Name)
	def.InitialStatus = strings.TrimSpace(def.InitialStatus)
	def.Statuses = append([]WorkflowStatus{}, def.Statuses...)
	for statusIndex := range def.Statuses {
		status := &def.Statuses[statusIndex]
		status.ID = strings.TrimSpace(status.ID)
		status.Join = strings.TrimSpace(status.Join)
		status.Requirements = append([]WorkflowRequirement{}, status.Requirements...)
		for requirementIndex := range status.Requirements {
			requirement := &status.Requirements[requirementIndex]
			requirement.ID = strings.TrimSpace(requirement.ID)
			requirement.Pool = strings.TrimSpace(requirement.Pool)
			requirement.Dispatch = strings.TrimSpace(requirement.Dispatch)
			requirement.Inputs = trimStrings(requirement.Inputs)
			requirement.Produces = trimStrings(requirement.Produces)
			requirement.Outcomes = trimStrings(requirement.Outcomes)
		}
		status.Transitions = append([]WorkflowTransition{}, status.Transitions...)
		for transitionIndex := range status.Transitions {
			status.Transitions[transitionIndex].When = strings.TrimSpace(status.Transitions[transitionIndex].When)
			status.Transitions[transitionIndex].To = strings.TrimSpace(status.Transitions[transitionIndex].To)
		}
	}
	def.Budgets.OnExhausted = strings.TrimSpace(def.Budgets.OnExhausted)
	def.Timeouts.Assignment = strings.TrimSpace(def.Timeouts.Assignment)
	def.Timeouts.Question = strings.TrimSpace(def.Timeouts.Question)
	def.Timeouts.OnTimeout = strings.TrimSpace(def.Timeouts.OnTimeout)
	def.Retries.Backoff = strings.TrimSpace(def.Retries.Backoff)
	def.Retries.OnExhausted = strings.TrimSpace(def.Retries.OnExhausted)
	def.Questions.RouteTo = strings.TrimSpace(def.Questions.RouteTo)
	def.Questions.AllowedHolds = trimStrings(def.Questions.AllowedHolds)
	def.Questions.Timeout = strings.TrimSpace(def.Questions.Timeout)
	def.Observations.OnLateEvent = strings.TrimSpace(def.Observations.OnLateEvent)
	def.Observations.AllowedReactions = trimStrings(def.Observations.AllowedReactions)
	def.Permissions.Tools = trimStrings(def.Permissions.Tools)
	def.Permissions.Channels.Subscribe = trimStrings(def.Permissions.Channels.Subscribe)
	def.Permissions.Channels.Reactions = trimStrings(def.Permissions.Channels.Reactions)
	expandPreviousOutputs(&def)
	return def
}

func trimStrings(values []string) []string {
	if values == nil {
		return []string{}
	}
	out := make([]string, len(values))
	for i, value := range values {
		out[i] = strings.TrimSpace(value)
	}
	return out
}

func expandPreviousOutputs(def *WorkflowDefinition) {
	statusPositions := make(map[string][]int, len(def.Statuses))
	for i, status := range def.Statuses {
		statusPositions[status.ID] = append(statusPositions[status.ID], i)
	}
	predecessors := make([][]int, len(def.Statuses))
	for from, status := range def.Statuses {
		for _, transition := range status.Transitions {
			positions := statusPositions[transition.To]
			if len(positions) == 1 {
				to := positions[0]
				predecessors[to] = append(predecessors[to], from)
			}
		}
	}
	for statusIndex := range def.Statuses {
		ancestors := make([]bool, len(def.Statuses))
		queue := append([]int(nil), predecessors[statusIndex]...)
		for len(queue) > 0 {
			candidate := queue[0]
			queue = queue[1:]
			if ancestors[candidate] {
				continue
			}
			ancestors[candidate] = true
			queue = append(queue, predecessors[candidate]...)
		}
		previous := make([]string, 0)
		seen := make(map[string]bool)
		for candidate, isAncestor := range ancestors {
			if !isAncestor {
				continue
			}
			for _, requirement := range def.Statuses[candidate].Requirements {
				for _, artifact := range requirement.Produces {
					if artifact != "" && !seen[artifact] {
						seen[artifact] = true
						previous = append(previous, artifact)
					}
				}
			}
		}
		for requirementIndex := range def.Statuses[statusIndex].Requirements {
			requirement := &def.Statuses[statusIndex].Requirements[requirementIndex]
			expanded := make([]string, 0, len(requirement.Inputs)+len(previous))
			seenInput := make(map[string]bool)
			for _, input := range requirement.Inputs {
				values := []string{input}
				if input == "previous_outputs" {
					values = previous
				}
				for _, value := range values {
					if value != "" && !seenInput[value] {
						seenInput[value] = true
						expanded = append(expanded, value)
					}
				}
			}
			requirement.Inputs = expanded
		}
	}
}

type workflowValidator struct {
	definition WorkflowDefinition
	errors     []WorkflowValidationError
	statuses   map[string][]int
	pools      map[string]struct{}
	artifacts  map[string]struct{}
}

func (v *workflowValidator) add(code, path, format string, args ...any) {
	v.errors = append(v.errors, WorkflowValidationError{
		Code: code, Path: path, Message: fmt.Sprintf(format, args...),
	})
}

func (v *workflowValidator) validate() {
	v.statuses = make(map[string][]int)
	v.pools = make(map[string]struct{})
	v.artifacts = make(map[string]struct{})
	if strings.TrimSpace(v.definition.Name) == "" {
		v.add("missing_workflow_name", "name", "workflow name is required")
	}
	if v.definition.Version <= 0 {
		v.add("invalid_workflow_version", "version", "workflow version must be positive")
	}

	for i, status := range v.definition.Statuses {
		path := fmt.Sprintf("statuses[%d]", i)
		statusID := strings.TrimSpace(status.ID)
		if statusID == "" {
			v.add("missing_status_id", path+".id", "status id is required")
			continue
		}
		if len(v.statuses[statusID]) > 0 {
			v.add("duplicate_status_id", path+".id", "status id %q is duplicated", statusID)
		}
		v.statuses[statusID] = append(v.statuses[statusID], i)
	}

	initial := strings.TrimSpace(v.definition.InitialStatus)
	if initial == "" || len(v.statuses[initial]) == 0 {
		v.add("missing_initial_status", "initial_status", "initial status must reference one status")
	} else if len(v.statuses[initial]) > 1 {
		v.add("multiple_initial_status", "initial_status", "initial status %q is declared more than once", initial)
	} else if v.definition.Statuses[v.statuses[initial][0]].Terminal {
		v.add("initial_status_terminal", "initial_status", "initial status must be nonterminal")
	}

	// Pools and artifacts are logical names declared by requirements. Inputs
	// count as externally supplied artifact declarations; produced names count
	// as workflow-generated declarations.
	for _, status := range v.definition.Statuses {
		for _, requirement := range status.Requirements {
			if pool := strings.TrimSpace(requirement.Pool); pool != "" {
				v.pools[pool] = struct{}{}
			}
			for _, name := range append(append([]string{}, requirement.Inputs...), requirement.Produces...) {
				if name = strings.TrimSpace(name); name != "" {
					v.artifacts[name] = struct{}{}
				}
			}
		}
	}

	for i, status := range v.definition.Statuses {
		v.validateStatus(i, status)
	}
	if backoff := v.definition.Retries.Backoff; backoff != "" && backoff != "immediate" {
		v.add("unsupported_retry_backoff", "retries.backoff",
			"retry backoff must be empty or immediate")
	}
	if action := v.definition.Timeouts.OnTimeout; action != "" && action != "retry" {
		v.add("unsupported_timeout_action", "timeouts.on_timeout",
			"timeout action must be empty or retry")
	}
	v.validateQuestionPool()
	v.validateChannelPatterns()
	v.validateReachability(initial)
}

func (v *workflowValidator) validateStatus(index int, status WorkflowStatus) {
	path := fmt.Sprintf("statuses[%d]", index)
	if status.Terminal && len(status.Requirements) != 0 {
		v.add("terminal_status_requirements", path+".requirements",
			"a terminal status cannot declare requirements")
	}
	if status.Terminal && len(status.Transitions) != 0 {
		v.add("terminal_status_transitions", path+".transitions",
			"a terminal status cannot declare outgoing transitions")
	}
	if !status.Terminal && len(status.Requirements) == 0 {
		v.add("empty_nonterminal_status", path+".requirements",
			"a nonterminal status must declare at least one requirement")
	}
	if status.Join != "" && status.Join != "require_all" {
		v.add("invalid_join", path+".join", "join must be empty or require_all")
	}
	requirements := make(map[string]WorkflowRequirement)
	for i, requirement := range status.Requirements {
		requirementPath := fmt.Sprintf("%s.requirements[%d]", path, i)
		id := strings.TrimSpace(requirement.ID)
		if id == "" {
			v.add("missing_requirement_id", requirementPath+".id", "requirement id is required")
		} else if strings.HasPrefix(id, questionRequirementPrefix) || strings.HasPrefix(id, observationRequirementPrefix) {
			v.add("reserved_requirement_id", requirementPath+".id", "requirement id uses a system-reserved prefix")
		} else if _, exists := requirements[id]; exists {
			v.add("duplicate_requirement_id", requirementPath+".id", "requirement id %q is duplicated", id)
		} else {
			requirements[id] = requirement
		}
		if strings.TrimSpace(requirement.Pool) == "" {
			v.add("unknown_pool", requirementPath+".pool", "requirement pool is required")
		}
		if requirement.Dispatch != DispatchClaimOne && requirement.Dispatch != DispatchRequireAll {
			v.add("invalid_dispatch", requirementPath+".dispatch",
				"dispatch must be claim_one or require_all")
		}
		seenOutcomes := make(map[string]struct{})
		if len(requirement.Outcomes) == 0 {
			v.add("invalid_outcome", requirementPath+".outcomes", "at least one outcome is required")
		}
		for outcomeIndex, outcome := range requirement.Outcomes {
			outcome = strings.TrimSpace(outcome)
			_, duplicate := seenOutcomes[outcome]
			if outcome == "" || duplicate {
				v.add("invalid_outcome", fmt.Sprintf("%s.outcomes[%d]", requirementPath, outcomeIndex),
					"outcomes must be non-empty and unique")
			}
			seenOutcomes[outcome] = struct{}{}
		}
	}

	unconditional := 0
	for i, transition := range status.Transitions {
		transitionPath := fmt.Sprintf("%s.transitions[%d]", path, i)
		destination := strings.TrimSpace(transition.To)
		if len(v.statuses[destination]) != 1 {
			v.add("unknown_transition_status", transitionPath+".to",
				"transition destination %q does not name exactly one status", destination)
		}
		guard := strings.TrimSpace(transition.When)
		if guard == "" {
			unconditional++
			continue
		}
		parser := newGuardParser(guard, requirements, v.artifacts)
		if _, err := parser.parse(); err != nil {
			code := err.code
			if code == "" {
				code = "unsupported_guard_token"
			}
			v.add(code, transitionPath+".when", "%s", err.message)
		}
	}
	if unconditional > 0 && len(status.Transitions) > 1 {
		v.add("ambiguous_unconditional_transitions", path+".transitions",
			"an unconditional transition must be the status's only transition")
	}
}

func (v *workflowValidator) validateQuestionPool() {
	pool := strings.TrimSpace(v.definition.Questions.RouteTo)
	if pool == "" {
		return
	}
	if _, ok := v.pools[pool]; !ok {
		v.add("unknown_pool", "questions.route_to", "question pool %q is not declared by a requirement", pool)
	}
}

var channelSegmentRE = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)

func (v *workflowValidator) validateChannelPatterns() {
	for i, pattern := range v.definition.Permissions.Channels.Subscribe {
		path := fmt.Sprintf("permissions.channels.subscribe[%d]", i)
		segments := strings.Split(pattern, ":")
		if len(segments) < 2 {
			v.add("invalid_channel_pattern", path, "channel pattern %q must contain at least two segments", pattern)
			continue
		}
		valid := true
		for segmentIndex, segment := range segments {
			if segment == "*" {
				valid = valid && len(segments) == 2 && segmentIndex == 1
				continue
			}
			if channelSegmentRE.MatchString(segment) {
				continue
			}
			if !strings.HasPrefix(segment, "${") || !strings.HasSuffix(segment, "}") {
				valid = false
				continue
			}
			field := strings.TrimSuffix(strings.TrimPrefix(segment, "${"), "}")
			switch field {
			case "task.key", "task.queue", "task.priority", "task.group", "task.customer":
			default:
				const artifactPrefix = "task.artifacts."
				if !strings.HasPrefix(field, artifactPrefix) {
					valid = false
				} else {
					name := strings.TrimPrefix(field, artifactPrefix)
					if _, exists := v.artifacts[name]; !exists {
						v.add("unknown_artifact", path, "channel pattern references unknown artifact %q", name)
						valid = false
					}
				}
			}
		}
		if !valid {
			v.add("invalid_channel_pattern", path, "channel pattern %q contains an unsafe expansion", pattern)
		}
	}
	for i, reaction := range v.definition.Observations.AllowedReactions {
		if !validObservationReaction(reaction) {
			v.add("invalid_observation_reaction", fmt.Sprintf("observations.allowed_reactions[%d]", i), "observation reaction %q is unsupported", reaction)
		}
	}
	for i, reaction := range v.definition.Permissions.Channels.Reactions {
		if !validObservationReaction(reaction) {
			v.add("invalid_observation_reaction", fmt.Sprintf("permissions.channels.reactions[%d]", i), "channel reaction %q is unsupported", reaction)
		}
	}
	if late := v.definition.Observations.OnLateEvent; late != "" && late != ObservationRecordOnly {
		v.add("invalid_late_observation_reaction", "observations.on_late_event", "late observations may only be recorded")
	}
}

func validObservationReaction(reaction string) bool {
	return reaction == ObservationRecordOnly || reaction == ObservationWakeCurrent || reaction == ObservationCreateRequirement || reaction == ObservationHoldAssignment
}

func (v *workflowValidator) validateReachability(initial string) {
	if len(v.statuses[initial]) != 1 {
		return
	}
	reachable := map[string]bool{initial: true}
	queue := []string{initial}
	for len(queue) > 0 {
		statusID := queue[0]
		queue = queue[1:]
		status := v.definition.Statuses[v.statuses[statusID][0]]
		for _, transition := range status.Transitions {
			next := strings.TrimSpace(transition.To)
			if len(v.statuses[next]) == 1 && !reachable[next] {
				reachable[next] = true
				queue = append(queue, next)
			}
		}
	}
	reachableTerminal := false
	for i, status := range v.definition.Statuses {
		id := strings.TrimSpace(status.ID)
		if id == "" || len(v.statuses[id]) != 1 {
			continue
		}
		if !reachable[id] {
			v.add("unreachable_status", fmt.Sprintf("statuses[%d]", i), "status %q is unreachable", id)
		} else if status.Terminal {
			reachableTerminal = true
		}
	}
	if !reachableTerminal {
		v.add("no_reachable_terminal", "statuses", "the initial status cannot reach a terminal status")
	}
}

type guardNode interface{ guardNode() }

type guardBinary struct {
	op          string
	left, right guardNode
}

func (guardBinary) guardNode() {}

type guardPredicate struct {
	kind, subject, value, operator string
}

func (guardPredicate) guardNode() {}

type guardTokenKind int

const (
	guardEOF guardTokenKind = iota
	guardIdentifier
	guardString
	guardDot
	guardLeftParen
	guardRightParen
	guardAnd
	guardOr
	guardEqual
	guardNotEqual
)

type guardToken struct {
	kind guardTokenKind
	text string
}

type guardParseError struct {
	code, message string
}

type guardParser struct {
	tokens       []guardToken
	position     int
	requirements map[string]WorkflowRequirement
	artifacts    map[string]struct{}
	lexErr       *guardParseError
}

func newGuardParser(source string, requirements map[string]WorkflowRequirement, artifacts map[string]struct{}) *guardParser {
	tokens, err := lexGuard(source)
	return &guardParser{tokens: tokens, requirements: requirements, artifacts: artifacts, lexErr: err}
}

func (p *guardParser) parse() (guardNode, *guardParseError) {
	if p.lexErr != nil {
		return nil, p.lexErr
	}
	node, err := p.parseOr()
	if err != nil {
		return nil, err
	}
	if p.peek().kind != guardEOF {
		return nil, unsupportedGuard("unsupported token %q", p.peek().text)
	}
	return node, nil
}

func (p *guardParser) parseOr() (guardNode, *guardParseError) {
	left, err := p.parseAnd()
	if err != nil {
		return nil, err
	}
	for p.match(guardOr) {
		right, err := p.parseAnd()
		if err != nil {
			return nil, err
		}
		left = guardBinary{op: "||", left: left, right: right}
	}
	return left, nil
}

func (p *guardParser) parseAnd() (guardNode, *guardParseError) {
	left, err := p.parsePrimary()
	if err != nil {
		return nil, err
	}
	for p.match(guardAnd) {
		right, err := p.parsePrimary()
		if err != nil {
			return nil, err
		}
		left = guardBinary{op: "&&", left: left, right: right}
	}
	return left, nil
}

func (p *guardParser) parsePrimary() (guardNode, *guardParseError) {
	if p.match(guardLeftParen) {
		node, err := p.parseOr()
		if err != nil {
			return nil, err
		}
		if !p.match(guardRightParen) {
			return nil, unsupportedGuard("missing closing parenthesis")
		}
		return node, nil
	}
	first, err := p.take(guardIdentifier, "predicate must start with an identifier")
	if err != nil {
		return nil, err
	}
	if first.text == "task" {
		return p.parseTaskPredicate()
	}
	if first.text == "artifact" || first.text == "artifacts" {
		return p.parseArtifactPredicate()
	}
	return p.parseRequirementPredicate(first.text)
}

func (p *guardParser) parseTaskPredicate() (guardNode, *guardParseError) {
	if !p.match(guardDot) {
		return nil, unsupportedGuard("task predicate must name a safe field")
	}
	field, err := p.take(guardIdentifier, "task predicate must name a safe field")
	if err != nil {
		return nil, err
	}
	safe := map[string]bool{
		"key": true, "queue": true, "priority": true, "status": true,
		"author": true, "customer": true, "group": true, "blocked": true,
	}
	if !safe[field.text] {
		return nil, unsupportedGuard("task field %q is not available to guards", field.text)
	}
	operator := ""
	if p.match(guardEqual) {
		operator = "=="
	} else if p.match(guardNotEqual) {
		operator = "!="
	} else {
		return nil, unsupportedGuard("task field %q requires == or !=", field.text)
	}
	value := p.peek()
	if value.kind != guardString && value.kind != guardIdentifier {
		return nil, unsupportedGuard("task comparison requires a literal")
	}
	p.position++
	return guardPredicate{kind: "task", subject: field.text, operator: operator, value: value.text}, nil
}

func (p *guardParser) parseArtifactPredicate() (guardNode, *guardParseError) {
	var name string
	if p.match(guardLeftParen) {
		token, err := p.take(guardIdentifier, "artifact() requires an artifact name")
		if err != nil {
			return nil, err
		}
		name = token.text
		if !p.match(guardRightParen) || !p.match(guardDot) {
			return nil, unsupportedGuard("artifact() predicate must end with .exists")
		}
	} else {
		if !p.match(guardDot) {
			return nil, unsupportedGuard("artifact predicate must name an artifact")
		}
		token, err := p.take(guardIdentifier, "artifact predicate must name an artifact")
		if err != nil {
			return nil, err
		}
		if token.text == "exists" && p.match(guardLeftParen) {
			artifact, err := p.take(guardIdentifier, "artifact.exists() requires an artifact name")
			if err != nil {
				return nil, err
			}
			name = artifact.text
			if !p.match(guardRightParen) {
				return nil, unsupportedGuard("artifact.exists() is missing a closing parenthesis")
			}
			return p.checkedArtifact(name)
		}
		name = token.text
		if !p.match(guardDot) {
			return nil, unsupportedGuard("artifact predicate must end with .exists")
		}
	}
	exists, err := p.take(guardIdentifier, "artifact predicate must end with .exists")
	if err != nil || exists.text != "exists" {
		return nil, unsupportedGuard("artifact predicate must end with .exists")
	}
	return p.checkedArtifact(name)
}

func (p *guardParser) checkedArtifact(name string) (guardNode, *guardParseError) {
	if _, exists := p.artifacts[name]; !exists {
		return nil, &guardParseError{code: "unknown_artifact", message: fmt.Sprintf("guard references unknown artifact %q", name)}
	}
	return guardPredicate{kind: "artifact_exists", subject: name}, nil
}

func (p *guardParser) parseRequirementPredicate(requirementID string) (guardNode, *guardParseError) {
	requirement, exists := p.requirements[requirementID]
	if !exists {
		if p.peek().kind == guardLeftParen {
			return nil, unsupportedGuard("function %q is not supported in guards", requirementID)
		}
		return nil, &guardParseError{code: "unknown_requirement", message: fmt.Sprintf("guard references unknown requirement %q", requirementID)}
	}
	if !p.match(guardDot) {
		return nil, unsupportedGuard("requirement predicate must name an outcome")
	}
	predicate, err := p.take(guardIdentifier, "requirement predicate must name an outcome")
	if err != nil {
		return nil, err
	}
	kind, outcome := "requirement_outcome", predicate.text
	if predicate.text == "all" || predicate.text == "any" {
		kind = predicate.text
		if !p.match(guardLeftParen) {
			return nil, unsupportedGuard("%s predicate requires an outcome", predicate.text)
		}
		outcomeToken, err := p.take(guardIdentifier, "outcome is required")
		if err != nil {
			return nil, err
		}
		outcome = outcomeToken.text
		if !p.match(guardRightParen) {
			return nil, unsupportedGuard("%s predicate is missing a closing parenthesis", predicate.text)
		}
	}
	for _, allowed := range requirement.Outcomes {
		if strings.TrimSpace(allowed) == outcome {
			return guardPredicate{kind: kind, subject: requirementID, value: outcome}, nil
		}
	}
	return nil, &guardParseError{code: "invalid_outcome", message: fmt.Sprintf(
		"outcome %q is not allowed for requirement %q", outcome, requirementID)}
}

func (p *guardParser) peek() guardToken {
	if p.position >= len(p.tokens) {
		return guardToken{kind: guardEOF}
	}
	return p.tokens[p.position]
}

func (p *guardParser) match(kind guardTokenKind) bool {
	if p.peek().kind != kind {
		return false
	}
	p.position++
	return true
}

func (p *guardParser) take(kind guardTokenKind, message string) (guardToken, *guardParseError) {
	token := p.peek()
	if token.kind != kind {
		return guardToken{}, unsupportedGuard("%s", message)
	}
	p.position++
	return token, nil
}

func unsupportedGuard(format string, args ...any) *guardParseError {
	return &guardParseError{code: "unsupported_guard_token", message: fmt.Sprintf(format, args...)}
}

func lexGuard(source string) ([]guardToken, *guardParseError) {
	tokens := make([]guardToken, 0, 8)
	for position := 0; position < len(source); {
		r := rune(source[position])
		if unicode.IsSpace(r) {
			position++
			continue
		}
		switch {
		case strings.HasPrefix(source[position:], "&&"):
			tokens = append(tokens, guardToken{kind: guardAnd, text: "&&"})
			position += 2
		case strings.HasPrefix(source[position:], "||"):
			tokens = append(tokens, guardToken{kind: guardOr, text: "||"})
			position += 2
		case strings.HasPrefix(source[position:], "=="):
			tokens = append(tokens, guardToken{kind: guardEqual, text: "=="})
			position += 2
		case strings.HasPrefix(source[position:], "!="):
			tokens = append(tokens, guardToken{kind: guardNotEqual, text: "!="})
			position += 2
		case source[position] == '.':
			tokens = append(tokens, guardToken{kind: guardDot, text: "."})
			position++
		case source[position] == '(':
			tokens = append(tokens, guardToken{kind: guardLeftParen, text: "("})
			position++
		case source[position] == ')':
			tokens = append(tokens, guardToken{kind: guardRightParen, text: ")"})
			position++
		case source[position] == '"':
			end := position + 1
			for end < len(source) {
				if source[end] == '\\' {
					end += 2
					continue
				}
				if source[end] == '"' {
					break
				}
				end++
			}
			if end >= len(source) {
				return nil, unsupportedGuard("unterminated string literal")
			}
			quoted := source[position : end+1]
			value, err := strconv.Unquote(quoted)
			if err != nil {
				return nil, unsupportedGuard("invalid string literal")
			}
			tokens = append(tokens, guardToken{kind: guardString, text: value})
			position = end + 1
		case unicode.IsLetter(r) || r == '_':
			end := position + 1
			for end < len(source) {
				next := rune(source[end])
				if !unicode.IsLetter(next) && !unicode.IsDigit(next) && next != '_' && next != '-' {
					break
				}
				end++
			}
			tokens = append(tokens, guardToken{kind: guardIdentifier, text: source[position:end]})
			position = end
		default:
			return nil, unsupportedGuard("unsupported token %q", string(source[position]))
		}
	}
	tokens = append(tokens, guardToken{kind: guardEOF})
	return tokens, nil
}
