package tui

import (
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/alvinunreal/lazyskills/internal/scan"
)

type detailProfileSample struct {
	name     string
	metadata time.Duration
	preview  time.Duration
	actions  time.Duration
	detail   time.Duration
	sync     time.Duration
	view     time.Duration
	total    time.Duration
}

type sourceProfileSample struct {
	name      string
	children  time.Duration
	detail    time.Duration
	sync      time.Duration
	view      time.Duration
	total     time.Duration
	childRows int
	lineCount int
}

type navProfileSample struct {
	selected int
	name     string
	update   time.Duration
	view     time.Duration
	total    time.Duration
}

func TestProfileRealSkillDetailTimings(t *testing.T) {
	if os.Getenv("LAZYSKILLS_PROFILE_DETAIL") == "" {
		t.Skip("set LAZYSKILLS_PROFILE_DETAIL=1 to profile real skill detail timings")
	}

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	cwd = strings.TrimSuffix(cwd, "/internal/tui")

	var snapshotDuration time.Duration
	var resultErr error
	start := time.Now()
	result, resultErr := scan.Snapshot(cwd)
	snapshotDuration = time.Since(start)
	if resultErr != nil {
		t.Fatal(resultErr)
	}
	if len(result.Skills) == 0 {
		t.Fatalf("no real skills found from %s", cwd)
	}

	m := newModel(cwd)
	m.result = result
	m.width = 120
	m.height = 40

	rowsStart := time.Now()
	rows := m.visibleRows()
	rowsDuration := time.Since(rowsStart)

	var skillRows []int
	for i, row := range rows {
		if !row.isHeader && row.skill != nil {
			skillRows = append(skillRows, i)
		}
	}
	if len(skillRows) == 0 {
		t.Fatal("no selectable skill rows found")
	}
	if len(skillRows) > 10 {
		skillRows = skillRows[:10]
	}

	t.Logf("snapshot=%s skills=%d visible_rows=%d visible_rows_time=%s sampled_skill_rows=%d", snapshotDuration, len(result.Skills), len(rows), rowsDuration, len(skillRows))

	samples := make([]detailProfileSample, 0, len(skillRows))
	for _, selected := range skillRows {
		m.selected = selected
		row := rows[selected]

		totalStart := time.Now()

		start = time.Now()
		_ = m.metadataLinesForRows(rows, 56)
		metadataDuration := time.Since(start)

		start = time.Now()
		_ = m.previewLinesForRows(rows, 56)
		previewDuration := time.Since(start)

		start = time.Now()
		_ = m.currentActions()
		actionsDuration := time.Since(start)

		start = time.Now()
		_ = m.detailLines(80)
		detailDuration := time.Since(start)

		start = time.Now()
		m.syncViewport()
		syncDuration := time.Since(start)

		start = time.Now()
		_ = m.View()
		viewDuration := time.Since(start)

		totalDuration := time.Since(totalStart)

		s := detailProfileSample{
			name:     row.skill.Name,
			metadata: metadataDuration,
			preview:  previewDuration,
			actions:  actionsDuration,
			detail:   detailDuration,
			sync:     syncDuration,
			view:     viewDuration,
			total:    totalDuration,
		}
		samples = append(samples, s)
		t.Logf("skill=%q metadata=%s preview=%s actions=%s detail=%s sync=%s view=%s total=%s", s.name, s.metadata, s.preview, s.actions, s.detail, s.sync, s.view, s.total)
	}

	logStageSummary(t, "metadata", samples, func(s detailProfileSample) time.Duration { return s.metadata })
	logStageSummary(t, "preview", samples, func(s detailProfileSample) time.Duration { return s.preview })
	logStageSummary(t, "actions", samples, func(s detailProfileSample) time.Duration { return s.actions })
	logStageSummary(t, "detail", samples, func(s detailProfileSample) time.Duration { return s.detail })
	logStageSummary(t, "sync", samples, func(s detailProfileSample) time.Duration { return s.sync })
	logStageSummary(t, "view", samples, func(s detailProfileSample) time.Duration { return s.view })
	logStageSummary(t, "total", samples, func(s detailProfileSample) time.Duration { return s.total })
}

func TestProfileRealSourceModalTimings(t *testing.T) {
	if os.Getenv("LAZYSKILLS_PROFILE_SOURCE") == "" {
		t.Skip("set LAZYSKILLS_PROFILE_SOURCE=1 to profile real source modal timings")
	}

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	cwd = strings.TrimSuffix(cwd, "/internal/tui")

	start := time.Now()
	result, err := scan.Snapshot(cwd)
	snapshotDuration := time.Since(start)
	if err != nil {
		t.Fatal(err)
	}

	m := newModel(cwd)
	m.result = result
	m.width = 120
	m.height = 40

	rows := m.visibleRows()
	var headerRows []int
	for i, row := range rows {
		if row.isHeader {
			headerRows = append(headerRows, i)
		}
	}
	if len(headerRows) == 0 {
		t.Fatal("no source header rows found")
	}
	if len(headerRows) > 10 {
		headerRows = headerRows[:10]
	}

	t.Logf("snapshot=%s skills=%d visible_rows=%d sampled_source_rows=%d", snapshotDuration, len(result.Skills), len(rows), len(headerRows))

	samples := make([]sourceProfileSample, 0, len(headerRows))
	for _, selected := range headerRows {
		m.selected = selected
		m.detailModal = true
		m.modalSource = rows[selected].groupName
		m.modalSelected = 0
		m.viewport.GotoTop()

		totalStart := time.Now()

		start = time.Now()
		children := m.modalChildRows(m.modalSource)
		childrenDuration := time.Since(start)

		start = time.Now()
		lines := m.sourceModalDetailLines(m.modalSource, 80)
		detailDuration := time.Since(start)

		start = time.Now()
		m.syncViewport()
		syncDuration := time.Since(start)

		start = time.Now()
		_ = m.View()
		viewDuration := time.Since(start)

		totalDuration := time.Since(totalStart)
		s := sourceProfileSample{
			name:      m.modalSource,
			children:  childrenDuration,
			detail:    detailDuration,
			sync:      syncDuration,
			view:      viewDuration,
			total:     totalDuration,
			childRows: len(children),
			lineCount: len(lines),
		}
		samples = append(samples, s)
		t.Logf("source=%q children=%s child_rows=%d detail=%s lines=%d sync=%s view=%s total=%s", s.name, s.children, s.childRows, s.detail, s.lineCount, s.sync, s.view, s.total)
	}

	logSourceStageSummary(t, "children", samples, func(s sourceProfileSample) time.Duration { return s.children })
	logSourceStageSummary(t, "detail", samples, func(s sourceProfileSample) time.Duration { return s.detail })
	logSourceStageSummary(t, "sync", samples, func(s sourceProfileSample) time.Duration { return s.sync })
	logSourceStageSummary(t, "view", samples, func(s sourceProfileSample) time.Duration { return s.view })
	logSourceStageSummary(t, "total", samples, func(s sourceProfileSample) time.Duration { return s.total })
}

func TestProfileRepeatedJNavigationTimings(t *testing.T) {
	if os.Getenv("LAZYSKILLS_PROFILE_NAV") == "" {
		t.Skip("set LAZYSKILLS_PROFILE_NAV=1 to profile repeated j navigation timings")
	}

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	cwd = strings.TrimSuffix(cwd, "/internal/tui")

	start := time.Now()
	result, err := scan.Snapshot(cwd)
	snapshotDuration := time.Since(start)
	if err != nil {
		t.Fatal(err)
	}

	m := newModel(cwd)
	m.result = result
	m.width = 120
	m.height = 40
	m.focus = focusSkills
	m.syncViewport()

	rows := m.visibleRows()
	steps := len(rows) - 1
	if steps > 40 {
		steps = 40
	}
	if steps <= 0 {
		t.Fatal("not enough visible rows to profile navigation")
	}

	t.Logf("snapshot=%s skills=%d visible_rows=%d steps=%d", snapshotDuration, len(result.Skills), len(rows), steps)

	samples := make([]navProfileSample, 0, steps)
	key := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}}

	for i := 0; i < steps; i++ {
		totalStart := time.Now()

		start = time.Now()
		updated, _ := m.Update(key)
		updateDuration := time.Since(start)
		m = updated.(appModel)

		start = time.Now()
		_ = m.View()
		viewDuration := time.Since(start)

		totalDuration := time.Since(totalStart)
		rows = m.visibleRows()
		name := ""
		if m.selected >= 0 && m.selected < len(rows) {
			row := rows[m.selected]
			if row.isHeader {
				name = "source:" + row.groupName
			} else if row.skill != nil {
				name = row.skill.Name
			}
		}
		s := navProfileSample{selected: m.selected, name: name, update: updateDuration, view: viewDuration, total: totalDuration}
		samples = append(samples, s)
		t.Logf("step=%d selected=%d item=%q update=%s view=%s total=%s", i+1, s.selected, s.name, s.update, s.view, s.total)
	}

	logNavStageSummary(t, "update", samples, func(s navProfileSample) time.Duration { return s.update })
	logNavStageSummary(t, "view", samples, func(s navProfileSample) time.Duration { return s.view })
	logNavStageSummary(t, "total", samples, func(s navProfileSample) time.Duration { return s.total })
}

func logNavStageSummary(t *testing.T, name string, samples []navProfileSample, pick func(navProfileSample) time.Duration) {
	t.Helper()
	values := make([]time.Duration, 0, len(samples))
	var sum time.Duration
	for _, s := range samples {
		v := pick(s)
		values = append(values, v)
		sum += v
	}
	sort.Slice(values, func(i, j int) bool { return values[i] < values[j] })
	avg := time.Duration(0)
	if len(values) > 0 {
		avg = sum / time.Duration(len(values))
	}
	t.Logf("summary stage=%s min=%s p50=%s max=%s avg=%s", name, values[0], values[len(values)/2], values[len(values)-1], avg)
}

func logSourceStageSummary(t *testing.T, name string, samples []sourceProfileSample, pick func(sourceProfileSample) time.Duration) {
	t.Helper()
	values := make([]time.Duration, 0, len(samples))
	var sum time.Duration
	for _, s := range samples {
		v := pick(s)
		values = append(values, v)
		sum += v
	}
	sort.Slice(values, func(i, j int) bool { return values[i] < values[j] })
	avg := time.Duration(0)
	if len(values) > 0 {
		avg = sum / time.Duration(len(values))
	}
	t.Logf("summary stage=%s min=%s p50=%s max=%s avg=%s", name, values[0], values[len(values)/2], values[len(values)-1], avg)
}

func logStageSummary(t *testing.T, name string, samples []detailProfileSample, pick func(detailProfileSample) time.Duration) {
	t.Helper()
	values := make([]time.Duration, 0, len(samples))
	var sum time.Duration
	for _, s := range samples {
		v := pick(s)
		values = append(values, v)
		sum += v
	}
	sort.Slice(values, func(i, j int) bool { return values[i] < values[j] })
	avg := time.Duration(0)
	if len(values) > 0 {
		avg = sum / time.Duration(len(values))
	}
	t.Logf("summary stage=%s min=%s p50=%s max=%s avg=%s", name, values[0], values[len(values)/2], values[len(values)-1], avg)
}
