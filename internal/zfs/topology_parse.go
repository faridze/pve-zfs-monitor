package zfs

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

var (
	poolHeaderRE = regexp.MustCompile(`(?m)^\s*pool:\s*`)
	scanErrorsRE = regexp.MustCompile(`(?i)with\s+(\d+)\s+errors?`)
)

// ParseStatus parses read-only `zpool status -v` text output.
func ParseStatus(output []byte) (*TopologyReport, error) {
	text := strings.TrimSpace(string(output))
	if text == "" {
		return nil, errors.New("empty zpool status output")
	}
	blocks := splitPoolBlocks(text)
	if len(blocks) == 0 {
		return nil, errors.New("no pool status found")
	}
	report := &TopologyReport{Pools: make([]PoolTopology, 0, len(blocks))}
	for _, block := range blocks {
		pool, err := parsePoolBlock(block)
		if err != nil {
			return nil, err
		}
		report.Pools = append(report.Pools, pool)
	}
	return report, nil
}

func splitPoolBlocks(text string) []string {
	indices := poolHeaderRE.FindAllStringIndex(text, -1)
	if len(indices) == 0 {
		return nil
	}
	blocks := make([]string, 0, len(indices))
	for i, idx := range indices {
		start := idx[0]
		end := len(text)
		if i+1 < len(indices) {
			end = indices[i+1][0]
		}
		blocks = append(blocks, strings.TrimSpace(text[start:end]))
	}
	return blocks
}

func parsePoolBlock(block string) (PoolTopology, error) {
	lines := strings.Split(block, "\n")
	if len(lines) == 0 {
		return PoolTopology{}, errors.New("empty pool block")
	}
	pool := PoolTopology{}
	if !strings.HasPrefix(strings.TrimSpace(lines[0]), "pool:") {
		return PoolTopology{}, errors.New("pool block missing pool header")
	}
	pool.Name = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(lines[0]), "pool:"))
	if pool.Name == "" {
		return PoolTopology{}, errors.New("pool name missing")
	}

	configStart := -1
	lastKey := ""
	for i := 1; i < len(lines); i++ {
		line := lines[i]
		trim := strings.TrimSpace(line)
		if trim == "" {
			continue
		}
		if strings.HasPrefix(trim, "config:") {
			configStart = i + 1
			break
		}
		if strings.HasPrefix(trim, "errors:") {
			pool.ErrorsLine = strings.TrimSpace(strings.TrimPrefix(trim, "errors:"))
			continue
		}
		if key, val, ok := splitMetaLine(trim); ok {
			lastKey = key
			applyMeta(&pool, key, val)
			continue
		}
		if lastKey != "" && isContinuationLine(line) {
			appendMeta(&pool, lastKey, trim)
		}
	}

	if configStart < 0 {
		return PoolTopology{}, fmt.Errorf("pool %q missing config section", pool.Name)
	}
	configLines := lines[configStart:]
	errorsIdx := len(configLines)
	for i, line := range configLines {
		if strings.HasPrefix(strings.TrimSpace(line), "errors:") {
			errorsIdx = i
			pool.ErrorsLine = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "errors:"))
			break
		}
	}
	configLines = configLines[:errorsIdx]
	root, issues, err := parseConfigTree(configLines)
	if err != nil {
		return PoolTopology{}, fmt.Errorf("pool %q config: %w", pool.Name, err)
	}
	pool.Root = root
	pool.ParseIssues = issues
	if pool.State == "" {
		return PoolTopology{}, fmt.Errorf("pool %q missing state", pool.Name)
	}
	if root == nil {
		return PoolTopology{}, fmt.Errorf("pool %q incomplete topology", pool.Name)
	}
	return pool, nil
}

func splitMetaLine(trim string) (string, string, bool) {
	idx := strings.Index(trim, ":")
	if idx <= 0 {
		return "", "", false
	}
	key := strings.ToLower(strings.TrimSpace(trim[:idx]))
	if !isMetaKey(key) {
		return "", "", false
	}
	return key, strings.TrimSpace(trim[idx+1:]), true
}

func isMetaKey(key string) bool {
	switch key {
	case "state", "status", "action", "see", "scan", "scrub":
		return true
	default:
		return false
	}
}

func isContinuationLine(line string) bool {
	if line == "" {
		return false
	}
	trim := strings.TrimSpace(line)
	if strings.Contains(trim, ":") {
		key := strings.ToLower(strings.TrimSpace(strings.Split(trim, ":")[0]))
		if isMetaKey(key) {
			return false
		}
	}
	r := rune(line[0])
	return r == ' ' || r == '\t'
}

func applyMeta(pool *PoolTopology, key, val string) {
	switch key {
	case "state":
		pool.State = val
	case "status":
		pool.StatusText = append(pool.StatusText, val)
	case "action":
		pool.ActionText = append(pool.ActionText, val)
	case "see":
		pool.SeeText = append(pool.SeeText, val)
	case "scan", "scrub":
		pool.Scan = parseScanLine(val)
	}
}

func appendMeta(pool *PoolTopology, key, val string) {
	switch key {
	case "status":
		if len(pool.StatusText) == 0 {
			pool.StatusText = []string{val}
			return
		}
		pool.StatusText[len(pool.StatusText)-1] += " " + val
	case "action":
		if len(pool.ActionText) == 0 {
			pool.ActionText = []string{val}
			return
		}
		pool.ActionText[len(pool.ActionText)-1] += " " + val
	case "see":
		if len(pool.SeeText) == 0 {
			pool.SeeText = []string{val}
			return
		}
		pool.SeeText[len(pool.SeeText)-1] += " " + val
	}
}

func parseScanLine(raw string) *ScanInfo {
	info := &ScanInfo{Raw: raw, Requested: true, Phase: "unknown", Operation: "unknown"}
	lower := strings.ToLower(raw)
	switch {
	case strings.Contains(lower, "none requested"):
		info.Phase = "none"
		info.Operation = "none"
		info.Requested = false
	case strings.Contains(lower, "scrub in progress"), strings.Contains(lower, "scan in progress"):
		info.Phase = "in_progress"
		info.Operation = "scrub"
	case strings.Contains(lower, "resilver in progress"):
		info.Phase = "in_progress"
		info.Operation = "resilver"
	case strings.Contains(lower, "scrub canceled"), strings.Contains(lower, "scrub cancelled"):
		info.Phase = "cancelled"
		info.Operation = "scrub"
	case strings.Contains(lower, "scrub repaired"), strings.Contains(lower, "scrub completed"):
		info.Phase = "completed"
		info.Operation = "scrub"
	case strings.Contains(lower, "resilvered"):
		info.Phase = "completed"
		info.Operation = "resilver"
	default:
		info.Requested = raw != ""
	}
	if m := scanErrorsRE.FindStringSubmatch(raw); len(m) == 2 {
		if n, err := strconv.ParseUint(m[1], 10, 64); err == nil {
			info.Errors = &n
		}
	}
	return info
}

type configRow struct {
	depth int
	node  VdevNode
}

func parseConfigTree(lines []string) (*VdevNode, []string, error) {
	var issues []string
	rows := make([]configRow, 0)
	minDepth := -1
	step := 0
	for _, line := range lines {
		trim := strings.TrimSpace(line)
		if trim == "" {
			continue
		}
		upper := strings.ToUpper(trim)
		if strings.HasPrefix(upper, "NAME ") && strings.Contains(upper, "STATE") {
			continue
		}
		depth, node, supplemental, err := parseConfigLine(line)
		if err != nil {
			return nil, issues, err
		}
		if supplemental {
			if len(rows) == 0 {
				return nil, issues, errors.New("device path without parent vdev")
			}
			rows[len(rows)-1].node.DevicePath = node.Name
			continue
		}
		if minDepth < 0 || depth < minDepth {
			minDepth = depth
		}
		rows = append(rows, configRow{depth: depth, node: node})
	}
	if len(rows) == 0 {
		return nil, issues, errors.New("config table empty")
	}
	depths := make([]int, 0, len(rows))
	for _, r := range rows {
		depths = append(depths, r.depth-minDepth)
	}
	step = detectIndentStep(depths)
	if step <= 0 {
		step = 1
	}
	levels := make([]int, len(rows))
	for i, d := range depths {
		if d%step != 0 {
			issues = append(issues, fmt.Sprintf("ambiguous indent on vdev %q", rows[i].node.Name))
		}
		levels[i] = d / step
	}
	var root *VdevNode
	stack := make([]*VdevNode, 0)
	for i, row := range rows {
		node := row.node
		node.VdevClass = classifyVdev(node.Name, levels[i])
		lvl := levels[i]
		if lvl == 0 {
			root = &node
			stack = []*VdevNode{root}
			rows[i].node = *root
			continue
		}
		if lvl > len(stack) {
			return nil, issues, fmt.Errorf("ambiguous topology near %q", node.Name)
		}
		parent := stack[lvl-1]
		parent.Children = append(parent.Children, node)
		child := &parent.Children[len(parent.Children)-1]
		if lvl < len(stack) {
			stack = stack[:lvl]
		}
		stack = append(stack, child)
	}
	if root == nil {
		return nil, issues, errors.New("missing pool root vdev")
	}
	return root, issues, nil
}

func detectIndentStep(depths []int) int {
	if len(depths) < 2 {
		return 1
	}
	minDiff := 0
	for i := 1; i < len(depths); i++ {
		d := depths[i] - depths[i-1]
		if d <= 0 {
			continue
		}
		if minDiff == 0 || d < minDiff {
			minDiff = d
		}
	}
	if minDiff == 0 {
		for _, d := range depths {
			if d > 0 {
				return d
			}
		}
		return 1
	}
	return minDiff
}

func parseConfigLine(line string) (int, VdevNode, bool, error) {
	depth := measureDepth(line)
	content := strings.TrimLeft(line, " \t")
	parts := strings.Fields(content)
	if len(parts) < 5 {
		return 0, VdevNode{}, false, fmt.Errorf("malformed config row: %q", strings.TrimSpace(line))
	}
	counterStart, ok := findCounterStart(parts)
	if !ok {
		return 0, VdevNode{}, false, fmt.Errorf("malformed counter row: %q", strings.TrimSpace(line))
	}
	stateIdx := counterStart - 1
	for stateIdx > 0 && !isVdevState(parts[stateIdx]) {
		stateIdx--
	}
	if stateIdx < 1 || !isVdevState(parts[stateIdx]) {
		return 0, VdevNode{}, false, fmt.Errorf("unknown state in row: %q", strings.TrimSpace(line))
	}
	name := strings.Join(parts[:stateIdx], " ")
	state := parts[stateIdx]
	detail := ""
	if counterStart+3 < len(parts) {
		detail = strings.Join(parts[counterStart+3:], " ")
	}
	read, readOK := parseCounterToken(parts[counterStart])
	write, writeOK := parseCounterToken(parts[counterStart+1])
	cksum, cksumOK := parseCounterToken(parts[counterStart+2])
	if strings.HasPrefix(name, "/dev/") {
		return depth, VdevNode{Name: name}, true, nil
	}
	node := VdevNode{Name: name, State: state, StateDetail: detail}
	if readOK {
		node.ReadErrors = read
	}
	if writeOK {
		node.WriteErrors = write
	}
	if cksumOK {
		node.CksumErrors = cksum
	}
	return depth, node, false, nil
}

func findCounterStart(parts []string) (int, bool) {
	for i := len(parts) - 3; i >= 1; i-- {
		if isDigits(parts[i]) && isDigits(parts[i+1]) && isDigits(parts[i+2]) {
			return i, true
		}
	}
	return 0, false
}

func isVdevState(s string) bool {
	switch s {
	case "ONLINE", "DEGRADED", "FAULTED", "OFFLINE", "UNAVAIL", "REMOVED", "SUSPENDED", "UNHEALTHY", "AVAIL":
		return true
	default:
		return false
	}
}

func measureDepth(line string) int {
	w := 0
	for _, r := range line {
		switch r {
		case '\t':
			w += 8
		case ' ':
			w++
		default:
			return w
		}
	}
	return w
}

func parseCounterToken(tok string) (*uint64, bool) {
	tok = strings.TrimSpace(tok)
	if tok == "" || tok == "-" {
		return nil, false
	}
	if !isDigits(tok) {
		return nil, false
	}
	n, err := strconv.ParseUint(tok, 10, 64)
	if err != nil {
		return nil, false
	}
	return &n, true
}

func isDigits(s string) bool {
	for _, r := range s {
		if !unicode.IsDigit(r) {
			return false
		}
	}
	return len(s) > 0
}

func classifyVdev(name string, level int) string {
	if level == 0 {
		return "pool"
	}
	lower := strings.ToLower(name)
	switch {
	case strings.HasPrefix(lower, "mirror"):
		return "mirror"
	case strings.HasPrefix(lower, "raidz3"):
		return "raidz"
	case strings.HasPrefix(lower, "raidz2"):
		return "raidz"
	case strings.HasPrefix(lower, "raidz"):
		return "raidz"
	case strings.HasPrefix(lower, "spare"):
		return "spare"
	case strings.HasPrefix(lower, "special"):
		return "special"
	case strings.HasPrefix(lower, "logs"):
		return "logs"
	case strings.HasPrefix(lower, "stripe"):
		return "stripe"
	case strings.HasPrefix(name, "/dev/"):
		return "disk"
	default:
		return "leaf"
	}
}

// EnrichListHealth annotates pools with list health and consistency flags.
func (r *TopologyReport) EnrichListHealth(list []Pool) {
	byName := make(map[string]string, len(list))
	for _, p := range list {
		byName[p.Name] = p.Health
	}
	for i := range r.Pools {
		health, ok := byName[r.Pools[i].Name]
		if !ok {
			r.Pools[i].ParseIssues = append(r.Pools[i].ParseIssues, "pool missing from zpool list")
			r.Pools[i].HealthMatch = boolPtr(false)
			continue
		}
		r.Pools[i].ListHealth = health
		match := strings.EqualFold(r.Pools[i].State, health)
		r.Pools[i].HealthMatch = &match
		if !match {
			r.Pools[i].ParseIssues = append(r.Pools[i].ParseIssues,
				fmt.Sprintf("state %q differs from list health %q", r.Pools[i].State, health))
		}
	}
}

func boolPtr(v bool) *bool { return &v }
