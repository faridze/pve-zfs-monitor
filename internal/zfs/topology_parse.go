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
	if err := validateTopologyReport(report); err != nil {
		return nil, err
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
	foundErrorsSection := pool.ErrorsLine != ""
	for i, line := range configLines {
		if strings.HasPrefix(strings.TrimSpace(line), "errors:") {
			errorsIdx = i
			pool.ErrorsLine = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "errors:"))
			foundErrorsSection = true
			break
		}
	}
	if !foundErrorsSection {
		return PoolTopology{}, fmt.Errorf("pool %q missing errors section", pool.Name)
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
	depth     int
	node      VdevNode
	isSection bool
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
		if section, ok := isAuxSectionHeader(trim); ok {
			rows = append(rows, configRow{
				depth:     measureDepth(line),
				isSection: true,
				node: VdevNode{
					Name:          section,
					VdevClass:     auxSectionClass(section),
					StateObserved: false,
				},
			})
			continue
		}
		depth, node, err := parseConfigLine(line)
		if err != nil {
			return nil, issues, err
		}
		if minDepth < 0 || depth < minDepth {
			minDepth = depth
		}
		rows = append(rows, configRow{depth: depth, node: node})
	}
	if len(rows) == 0 {
		return nil, issues, errors.New("config table empty")
	}
	firstSectionIdx := len(rows)
	for i, r := range rows {
		if r.isSection {
			firstSectionIdx = i
			break
		}
	}
	dataDepths := make([]int, 0, len(rows))
	depths := make([]int, 0, len(rows))
	for i, r := range rows {
		if r.isSection {
			depths = append(depths, -1)
			continue
		}
		rel := r.depth - minDepth
		depths = append(depths, rel)
		if i < firstSectionIdx {
			dataDepths = append(dataDepths, rel)
		}
	}
	step = detectIndentStep(dataDepths)
	if step <= 0 {
		step = 1
	}
	levels := make([]int, len(rows))
	for i, d := range depths {
		if rows[i].isSection {
			levels[i] = -1
			continue
		}
		if i > firstSectionIdx {
			levels[i] = -2
			continue
		}
		if d%step != 0 {
			return nil, issues, fmt.Errorf("ambiguous indent on vdev %q", rows[i].node.Name)
		}
		levels[i] = d / step
	}
	var root *VdevNode
	stack := make([]*VdevNode, 0)
	inSection := false
	sectionBaseDepth := -1
	auxMinDepth := -1
	auxRelDepths := make([]int, 0, 8)
	for i, row := range rows {
		node := row.node
		lvl := levels[i]
		if row.isSection {
			if root == nil {
				return nil, issues, errors.New("auxiliary section before pool root")
			}
			node.VdevClass = auxSectionClass(node.Name)
			root.Children = append(root.Children, node)
			sectionNode := &root.Children[len(root.Children)-1]
			stack = []*VdevNode{root, sectionNode}
			inSection = true
			sectionBaseDepth = row.depth
			auxMinDepth = -1
			auxRelDepths = auxRelDepths[:0]
			continue
		}
		if inSection {
			if row.depth <= sectionBaseDepth {
				return nil, issues, fmt.Errorf("ambiguous auxiliary hierarchy near %q", node.Name)
			}
			if auxMinDepth < 0 {
				auxMinDepth = row.depth
			}
			rel := row.depth - auxMinDepth
			auxRelDepths = append(auxRelDepths, rel)
			auxStep := detectIndentStep(auxRelDepths)
			if auxStep <= 0 {
				auxStep = 1
			}
			if rel%auxStep != 0 {
				return nil, issues, fmt.Errorf("ambiguous auxiliary indent on %q", node.Name)
			}
			auxLevel := rel / auxStep
			parentIdx := auxLevel + 1
			if parentIdx >= len(stack) {
				return nil, issues, fmt.Errorf("invalid auxiliary depth for %q", node.Name)
			}
			node.VdevClass = classifyVdev(node.Name, auxLevel+1)
			parent := stack[parentIdx]
			parent.Children = append(parent.Children, node)
			child := &parent.Children[len(parent.Children)-1]
			stack = stack[:parentIdx+1]
			stack = append(stack, child)
			continue
		}
		node.VdevClass = classifyVdev(node.Name, lvl)
		if lvl == 0 {
			if root != nil {
				return nil, issues, fmt.Errorf("multiple root vdev rows (%q and %q)", root.Name, node.Name)
			}
			root = &node
			stack = []*VdevNode{root}
			rows[i].node = *root
			continue
		}
		if root == nil {
			return nil, issues, fmt.Errorf("child vdev %q before pool root", node.Name)
		}
		if lvl > len(stack) {
			return nil, issues, fmt.Errorf("invalid parent depth for %q", node.Name)
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

func parseConfigLine(line string) (int, VdevNode, error) {
	depth := measureDepth(line)
	content := strings.TrimLeft(line, " \t")
	parts := strings.Fields(content)
	if len(parts) == 2 && isVdevState(parts[1]) {
		return depth, VdevNode{
			Name:          parts[0],
			State:         parts[1],
			StateObserved: true,
			VdevClass:     classifyVdev(parts[0], 1),
		}, nil
	}
	if len(parts) >= 2 && isCounterlessSpareState(parts[1]) {
		return depth, VdevNode{
			Name:          parts[0],
			State:         parts[1],
			StateDetail:   strings.Join(parts[2:], " "),
			StateObserved: true,
			VdevClass:     classifyVdev(parts[0], 1),
		}, nil
	}
	if len(parts) < 5 {
		return 0, VdevNode{}, fmt.Errorf("malformed config row: %q", strings.TrimSpace(line))
	}
	counterStart, ok := findCounterStart(parts)
	if !ok {
		return 0, VdevNode{}, fmt.Errorf("malformed counter row: %q", strings.TrimSpace(line))
	}
	stateIdx := counterStart - 1
	for stateIdx > 0 && !isVdevState(parts[stateIdx]) {
		stateIdx--
	}
	if stateIdx < 1 || !isVdevState(parts[stateIdx]) {
		return 0, VdevNode{}, fmt.Errorf("unknown state in row: %q", strings.TrimSpace(line))
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
	node := VdevNode{Name: name, State: state, StateDetail: detail, StateObserved: true}
	if readOK {
		node.ReadErrors = read
	}
	if writeOK {
		node.WriteErrors = write
	}
	if cksumOK {
		node.CksumErrors = cksum
	}
	return depth, node, nil
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
	case "ONLINE", "DEGRADED", "FAULTED", "OFFLINE", "UNAVAIL", "REMOVED", "SUSPENDED", "UNHEALTHY", "AVAIL", "INUSE":
		return true
	default:
		return false
	}
}

func isCounterlessSpareState(s string) bool {
	return s == "AVAIL" || s == "INUSE"
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
	case strings.HasPrefix(lower, "draid"):
		return "unknown"
	case strings.HasPrefix(lower, "spare"):
		return "spare"
	case strings.HasPrefix(lower, "special"):
		return "special"
	case strings.HasPrefix(lower, "logs"):
		return "logs"
	case strings.HasPrefix(lower, "cache"):
		return "cache"
	case strings.HasPrefix(lower, "stripe"):
		return "stripe"
	case strings.HasPrefix(name, "/dev/"):
		return "disk"
	default:
		return "leaf"
	}
}

// MarkListHealthUnverified marks every pool as unverified when list output is unavailable.
func (r *TopologyReport) MarkListHealthUnverified(reason string) {
	for i := range r.Pools {
		r.Pools[i].ListHealthStatus = ListHealthUnverified
		r.Pools[i].ParseIssues = append(r.Pools[i].ParseIssues, reason)
	}
	r.ListComparisonStatus = ReportListComparisonUnverified
}

// EnrichListHealth compares status pools with zpool list in both directions.
func (r *TopologyReport) EnrichListHealth(list []Pool) {
	byName := make(map[string]string, len(list))
	for _, p := range list {
		byName[p.Name] = p.Health
	}
	statusNames := make(map[string]bool, len(r.Pools))
	for i := range r.Pools {
		statusNames[r.Pools[i].Name] = true
		health, ok := byName[r.Pools[i].Name]
		if !ok {
			r.Pools[i].ListHealthStatus = ListHealthVerifiedInconsistent
			r.Pools[i].ParseIssues = append(r.Pools[i].ParseIssues, "pool missing from zpool list")
			continue
		}
		r.Pools[i].ListHealth = health
		if strings.EqualFold(r.Pools[i].State, health) {
			r.Pools[i].ListHealthStatus = ListHealthVerifiedConsistent
		} else {
			r.Pools[i].ListHealthStatus = ListHealthVerifiedInconsistent
			r.Pools[i].ParseIssues = append(r.Pools[i].ParseIssues,
				fmt.Sprintf("state %q differs from list health %q", r.Pools[i].State, health))
		}
	}
	for _, p := range list {
		if !statusNames[p.Name] {
			r.ReportIssues = append(r.ReportIssues,
				fmt.Sprintf("pool %q present in zpool list but missing from zpool status", p.Name))
		}
	}
	r.ComputeListComparisonStatus()
}

// ComputeListComparisonStatus summarizes cross-source pool completeness.
func (r *TopologyReport) ComputeListComparisonStatus() {
	if len(r.Pools) == 0 {
		r.ListComparisonStatus = ReportListComparisonUnverified
		return
	}
	unverified := 0
	inconsistent := 0
	for _, pool := range r.Pools {
		switch pool.ListHealthStatus {
		case ListHealthUnverified, "":
			unverified++
		case ListHealthVerifiedInconsistent:
			inconsistent++
		}
	}
	if unverified > 0 {
		r.ListComparisonStatus = ReportListComparisonUnverified
		return
	}
	if inconsistent > 0 || len(r.ReportIssues) > 0 {
		r.ListComparisonStatus = ReportListComparisonIncomplete
		if inconsistent > 0 {
			r.ListComparisonStatus = ReportListComparisonInconsistent
		}
		return
	}
	r.ListComparisonStatus = ReportListComparisonComplete
}
