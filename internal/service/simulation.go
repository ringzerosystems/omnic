package service

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
)

// FailRule defines a declarative failure injection rule.
type FailRule struct {
	Message    string `json:"message"`
	MatchTo    string `json:"match_to"`
	MatchFrom  string `json:"match_from"`
	Operation  string `json:"operation"`
	StatusCode int    `json:"status_code"`
	ErrorCode  int    `json:"error_code"`
	EveryN     int    `json:"every_n"`
	AfterN     int    `json:"after_n"`
	counter    int
	Timeout    bool `json:"timeout"`
}

// SimulationService manages dynamic failure rules for chaos and resilience testing.
type SimulationService struct {
	rules []FailRule
	mu    sync.Mutex
}

// NewSimulationService constructs an empty SimulationService.
func NewSimulationService() *SimulationService {
	return &SimulationService{
		rules: make([]FailRule, 0),
	}
}

// SetRules replaces all active failure rules.
func (s *SimulationService) SetRules(rules []FailRule) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rules = make([]FailRule, len(rules))
	copy(s.rules, rules)
}

// AddRule appends an active failure rule.
func (s *SimulationService) AddRule(rule FailRule) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rules = append(s.rules, rule)
}

// GetRules returns a copy of active failure rules.
func (s *SimulationService) GetRules() []FailRule {
	s.mu.Lock()
	defer s.mu.Unlock()
	res := make([]FailRule, len(s.rules))
	copy(res, s.rules)
	return res
}

// ClearRules removes all active failure rules.
func (s *SimulationService) ClearRules() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rules = make([]FailRule, 0)
}

// ParseRuleString parses a rule string in format `op=status:code[@cond]`.
func (s *SimulationService) ParseRuleString(ruleStr string) (FailRule, error) {
	var rule FailRule
	ruleStr = strings.TrimSpace(ruleStr)
	if ruleStr == "" {
		return rule, fmt.Errorf("empty rule string")
	}

	mainPart, condPart, hasCond := strings.Cut(ruleStr, "@")
	op, action, found := strings.Cut(mainPart, "=")
	if !found {
		return rule, fmt.Errorf("missing '=' in rule: %s", mainPart)
	}

	rule.Operation = strings.TrimSpace(strings.ToLower(op))
	action = strings.TrimSpace(action)
	if strings.EqualFold(action, "timeout") {
		rule.Timeout = true
		rule.StatusCode = 504
		rule.ErrorCode = 11200
	} else {
		statusStr, codeStr, foundCode := strings.Cut(action, ":")
		if !foundCode {
			return rule, fmt.Errorf("invalid action format, expected status:code: %s", action)
		}
		st, err := strconv.Atoi(statusStr)
		if err != nil {
			return rule, fmt.Errorf("invalid status code: %s", statusStr)
		}
		ec, err := strconv.Atoi(codeStr)
		if err != nil {
			return rule, fmt.Errorf("invalid error code: %s", codeStr)
		}
		rule.StatusCode = st
		rule.ErrorCode = ec
	}

	if hasCond {
		if err := parseRuleCondition(&rule, condPart); err != nil {
			return rule, err
		}
	}
	return rule, nil
}

func parseRuleCondition(rule *FailRule, cond string) error {
	cond = strings.TrimSpace(cond)
	switch {
	case strings.HasPrefix(cond, "to "):
		rule.MatchTo = strings.TrimSpace(strings.TrimPrefix(cond, "to "))
	case strings.HasPrefix(cond, "from "):
		rule.MatchFrom = strings.TrimSpace(strings.TrimPrefix(cond, "from "))
	case strings.HasPrefix(cond, "after "):
		n, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(cond, "after ")))
		if err != nil {
			return err
		}
		rule.AfterN = n
	case strings.HasPrefix(cond, "every "):
		n, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(cond, "every ")))
		if err != nil {
			return err
		}
		rule.EveryN = n
	default:
		return fmt.Errorf("unrecognized condition: %s", cond)
	}
	return nil
}

// Evaluate checks if any active rule triggers for the given operation and numbers.
func (s *SimulationService) Evaluate(op, to, from string) *TwilioError {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i := range s.rules {
		r := &s.rules[i]
		if !ruleMatches(r, op, to, from) {
			continue
		}
		r.counter++
		if r.AfterN > 0 && r.counter <= r.AfterN {
			continue
		}
		if r.EveryN > 0 && r.counter%r.EveryN != 0 {
			continue
		}
		msg := r.Message
		if msg == "" {
			msg = defaultErrorMessage(r.ErrorCode)
		}
		return NewTwilioError(r.ErrorCode, r.StatusCode, msg)
	}
	return nil
}

// ShouldTimeout checks if a timeout failure rule triggers for the operation.
func (s *SimulationService) ShouldTimeout(op, to, from string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i := range s.rules {
		r := &s.rules[i]
		if r.Timeout && ruleMatches(r, op, to, from) {
			r.counter++
			if r.AfterN > 0 && r.counter <= r.AfterN {
				continue
			}
			if r.EveryN > 0 && r.counter%r.EveryN != 0 {
				continue
			}
			return true
		}
	}
	return false
}

func ruleMatches(r *FailRule, op, to, from string) bool {
	if r.Operation != "" && !strings.EqualFold(r.Operation, op) {
		return false
	}
	if r.MatchTo != "" && r.MatchTo != to {
		return false
	}
	if r.MatchFrom != "" && r.MatchFrom != from {
		return false
	}
	return true
}

func defaultErrorMessage(code int) string {
	switch code {
	case 20429:
		return "Too Many Requests - Rate limit exceeded"
	case 30007:
		return "Carrier violation - Message filtered as spam"
	case 30003:
		return "Handset delivery failure - Destination unreachable"
	case 21610:
		return "Attempt to send to unsubscribed recipient"
	case 11200:
		return "HTTP retrieval failure"
	default:
		return "Simulated carrier error"
	}
}
