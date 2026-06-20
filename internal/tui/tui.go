package tui

import (
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/alvinunreal/lazyskills/internal/actions"
	"github.com/alvinunreal/lazyskills/internal/model"
	"github.com/alvinunreal/lazyskills/internal/runner"
	"github.com/alvinunreal/lazyskills/internal/scan"
)

type scopeFilter int

const (
	scopeAll scopeFilter = iota
	scopeProject
	scopeGlobal
)

type focusState int

const (
	focusSkills focusState = iota
	focusMetadata
	focusPreview
)

type DiscoveryStatus string

const (
	DiscoveryLoading DiscoveryStatus = "loading"
	DiscoveryReady   DiscoveryStatus = "ready"
	DiscoveryFailed  DiscoveryStatus = "failed"
)

type DiscoveredSkill struct {
	Name        string
	Description string
	Source      string
	SkillPath   string
	Preview     string
}

type SourceDiscovery struct {
	Status    DiscoveryStatus
	Skills    []DiscoveredSkill
	Error     string
	ScannedAt time.Time
}

type appModel struct {
	cwd              string
	result           model.ScanResult
	err              error
	selected         int
	filter           scopeFilter
	agent            string
	search           string
	searching        bool
	commands         bool
	selectedKeys     map[string]bool
	action           int
	confirming       bool
	confirmInput     string
	confirmError     string
	running          bool
	runningTitle     string
	actionResult     *runner.Result
	width            int
	height           int
	viewport         viewport.Model
	metadataViewport viewport.Model
	previewViewport  viewport.Model
	detailsFocused   bool
	detailModal      bool
	helpOpen         bool
	focus            focusState
	collapsedGroups  map[string]bool
	discovery        map[string]SourceDiscovery
	modalSelected    int
	modalSource      string
	pendingG         bool                    // saw a lone "g"; a second "g" jumps to top
	pendingAction    *actions.CommandPreview // action awaiting confirm (decoupled from selection)
}

type paneLayout struct {
	OuterWidth    int
	OuterHeight   int
	StyleWidth    int
	StyleHeight   int
	ContentWidth  int
	ContentHeight int
}

type appLayout struct {
	Small  bool
	Width  int
	Height int
	Left   paneLayout
	List   paneLayout
	Detail paneLayout
}

const (
	minLayoutWidth  = 40
	minLayoutHeight = 7
	appVersion      = "v1"
)

var (
	titleStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212"))
	borderStyle   = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1)
	selectedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("230")).Background(lipgloss.Color("62"))
	dimStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	errorStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
	warningStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))

	// Scope tags: project=cyan, global=magenta.
	scopeProjectStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("45"))
	scopeGlobalStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("213"))

	runExec  = runner.OSRunner{}.Run
	gitClone = defaultGitClone

	// Action Mode UI Polish Styles
	actionTitleStyle       = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("230")).Background(lipgloss.Color("62")).Padding(0, 1)
	activeActionStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("230")).Background(lipgloss.Color("62"))
	activeActionTitleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("230")).Background(lipgloss.Color("62"))
	activeActionSubStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("250")).Background(lipgloss.Color("62"))
	normalActionTitleStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("255"))
	normalActionSubStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("243"))
	actionNormalStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("255"))
	actionBorderColor      = lipgloss.Color("62")
	runningStyle           = lipgloss.NewStyle().Foreground(lipgloss.Color("208")).Bold(true)
	successStyle           = lipgloss.NewStyle().Foreground(lipgloss.Color("114")).Bold(true)

	// Metadata / Details styling
	metaKeyStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("243"))
	sectionHeaderStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("117"))
	healthHeaderStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("203"))
)

type snapshotMsg struct {
	result model.ScanResult
	err    error
}

type discoveryResultMsg struct {
	groupName string
	skills    []DiscoveredSkill
	err       error
}

type actionResultMsg struct {
	result         runner.Result
	mutates        bool
	partialSuccess bool
}

func Run(cwd string) error {
	program := tea.NewProgram(newModel(cwd), tea.WithAltScreen())
	_, err := program.Run()
	return err
}

func newModel(cwd string) appModel {
	return appModel{
		cwd:              cwd,
		viewport:         viewport.New(0, 0),
		metadataViewport: viewport.New(0, 0),
		previewViewport:  viewport.New(0, 0),
		collapsedGroups:  make(map[string]bool),
		discovery:        make(map[string]SourceDiscovery),
	}
}

func (m appModel) Init() tea.Cmd {
	return loadSnapshot(m.cwd)
}

func loadSnapshot(cwd string) tea.Cmd {
	return func() tea.Msg {
		result, err := scan.Snapshot(cwd)
		return snapshotMsg{result: result, err: err}
	}
}
