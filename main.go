package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/faiface/beep"
	"github.com/faiface/beep/mp3"
	"github.com/faiface/beep/speaker"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type keyMap struct {
	Up    key.Binding
	Down  key.Binding
	Pause key.Binding
	Reset key.Binding
	Help  key.Binding
	Quit  key.Binding
}

type model struct {
	choices       []string
	cursor        int
	lockedIn      bool
	paused        bool
	timer         Timer
	currentMode   int
	continueModal bool
	manualInput   bool
	progress      progress.Model
	textInput     textinput.Model

	keys keyMap
	help help.Model
}

type Timer struct {
	totalFocusTime     time.Duration
	remainingFocusTime time.Duration
	totalBreakTime     time.Duration
	remainingBreakTime time.Duration
}

type tickMsg time.Time

var (
	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#f01e1e")).
			Padding(0, 1)

	choiceStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#8c8f8e"))
)

func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Up, k.Down, k.Pause, k.Reset, k.Help, k.Quit}
}

func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down},
		{k.Pause, k.Reset},
		{k.Help, k.Quit},
	}
}

func initialModel() model {
	ti := textinput.New()
	ti.Placeholder = "focus_time/break_time"
	ti.Focus()
	ti.CharLimit = 156
	ti.Width = 20

	p := progress.New(progress.WithGradient("#e43543", "#e61631"))
	return model{
		textInput: ti,
		choices:   []string{"35/7", "60/15", "120/35", "input"},
		progress:  p,
		help:      help.New(),
		keys: keyMap{
			Quit:  key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
			Up:    key.NewBinding(key.WithKeys("↑/k"), key.WithHelp("↑/k", "move up")),
			Down:  key.NewBinding(key.WithKeys("↓/j"), key.WithHelp("↓/j", "move down")),
			Reset: key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "reset")),
			Pause: key.NewBinding(key.WithKeys("p"), key.WithHelp("p", "pause")),
		},
	}
}

func (m model) Init() tea.Cmd {
	return tea.Every(time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.progress.Width = msg.Width - 70
		m.help.Width = msg.Width
		return m, nil

	case progress.FrameMsg:
		progressModel, cmd := m.progress.Update(msg)
		m.progress = progressModel.(progress.Model)
		return m, cmd

	case tickMsg:
		if m.lockedIn {
			tickCmd := tickTimer()
			if m.currentMode == 0 {
				if m.timer.remainingFocusTime > 0 {
					m.timer.remainingFocusTime -= time.Second

					totalDuration := m.timer.totalFocusTime
					remainingDuration := m.timer.remainingFocusTime
					p := calculatePerc(totalDuration, remainingDuration)

					progressCmd := m.progress.SetPercent(p)

					return m, tea.Batch(progressCmd, tickCmd)
				} else {
					m.currentMode = 1

					m.lockedIn = false
					m.paused = true
					m.continueModal = true

					progressCmd := m.progress.SetPercent(0)

					playCmd := playSound()

					return m, tea.Batch(playCmd, progressCmd, tickCmd)
				}
			} else {
				if m.timer.remainingBreakTime > 0 {
					m.timer.remainingBreakTime -= time.Second

					totalDuration := m.timer.totalBreakTime
					remainingDuration := m.timer.remainingBreakTime
					p := calculatePerc(totalDuration, remainingDuration)

					progressCmd := m.progress.SetPercent(p)

					return m, tea.Batch(progressCmd, tickCmd)
				} else {
					m.currentMode = 0

					m.timer.remainingFocusTime = m.timer.totalFocusTime
					m.timer.remainingBreakTime = m.timer.totalBreakTime

					m.lockedIn = false
					m.paused = true
					m.continueModal = true
					progressCmd := m.progress.SetPercent(0)

					playCmd := playSound()

					return m, tea.Batch(playCmd, progressCmd, tickCmd)
				}
			}
		}

		return m, tickTimer()
	case tea.KeyMsg:

		switch {

		case key.Matches(msg, m.keys.Quit):
			return m, tea.Quit

		case msg.String() == "up" || msg.String() == "k":
			if m.cursor > 0 {
				m.cursor--
			}

		case msg.String() == "down" || msg.String() == "j":
			if !m.continueModal {
				if m.cursor < len(m.choices)-1 {
					m.cursor++
				}
			} else {
				if m.cursor < 1 {
					m.cursor++
				}
			}

		case key.Matches(msg, m.keys.Reset):
			m.currentMode = 0
			m.lockedIn = false
			m.paused = false
			m.manualInput = false

			m.timer.totalFocusTime = time.Duration(0)
			m.timer.remainingFocusTime = time.Duration(0)

			m.timer.totalBreakTime = time.Duration(0)
			m.timer.remainingBreakTime = time.Duration(0)

			progressCmd := m.progress.SetPercent(0)

			return m, progressCmd

		case key.Matches(msg, m.keys.Pause):
			m.paused = !m.paused
			m.lockedIn = !m.lockedIn

		case msg.String() == "enter" || msg.String() == " ":
			if !m.continueModal {
				pomodoroChoice := m.choices[m.cursor]
				if pomodoroChoice != "input" {
					m.cursor = 0
					m.lockedIn = true
					return m, m.startTimer(pomodoroChoice)
				} else {
					m.manualInput = !m.manualInput

					if !m.manualInput {
						m.cursor = 0
						m.lockedIn = true
						return m, m.startTimer(m.textInput.Value())
					}
				}

				return m, nil
			} else {
				if m.cursor == 0 {
					m.lockedIn = true
					m.paused = false
				} else {
					m.currentMode = 0
					m.lockedIn = false
					m.paused = false
				}
				m.continueModal = false
			}

		}
	}

	var cmd tea.Cmd

	if m.manualInput {
		if key, ok := msg.(tea.KeyMsg); ok && key.String() == "enter" {
			m.startTimer(m.textInput.Value())
			m.manualInput = false
			// m.paused = false
			return m, nil
		}
		m.textInput, cmd = m.textInput.Update(msg)
		return m, cmd
	}

	return m, nil
}

func calculatePerc(totalDuration, remainingDuration time.Duration) float64 {
	elapsed := totalDuration - remainingDuration
	p := (float64(elapsed) / float64(totalDuration))

	return p
}

func tickTimer() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func (m *model) startTimer(choice string) tea.Cmd {
	if !strings.Contains(choice, "/") {
		return nil
	}

	if choice == "input" {
		m.manualInput = true
	}

	splitChoice := strings.Split(choice, "/")
	focusTime, _ := strconv.Atoi(splitChoice[0])
	breakTime, _ := strconv.Atoi(splitChoice[1])

	m.timer.totalFocusTime = time.Duration(focusTime) * time.Minute
	m.timer.remainingFocusTime = time.Duration(focusTime) * time.Minute

	m.timer.totalBreakTime = time.Duration(breakTime) * time.Minute
	m.timer.remainingBreakTime = time.Duration(breakTime) * time.Minute

	return nil
}

func playSound() tea.Cmd {
	return func() tea.Msg {
		f, err := os.Open("sound.mp3")
		if err != nil {
			return nil
		}

		streamer, format, err := mp3.Decode(f)
		if err != nil {
			f.Close()
			return nil
		}

		_ = speaker.Init(format.SampleRate, format.SampleRate.N(time.Second/10))

		speaker.Play(beep.Seq(streamer, beep.Callback(func() {
			streamer.Close()
			f.Close()
		})))

		return nil
	}
}

func (m model) View() string {
	s := ""

	if !m.lockedIn && !m.continueModal && !m.paused && !m.manualInput {
		s += headerStyle.Render("Choose a pomodoro cycle") + "\n\n"
		for i, choice := range m.choices {

			cursor := " "
			if m.cursor == i {
				cursor = ">"
			}

			s += fmt.Sprintf("%s %s\n", cursor, choiceStyle.Render(choice))
		}
	}

	if m.manualInput {
		s += "\nFormat - focus_time/break_time: " + m.textInput.View()
	}

	if m.lockedIn || (m.paused && !m.lockedIn) && !m.continueModal {
		s += "LOCKED IN\n\n"
		if m.currentMode == 0 {
			remainingFocusTime := m.timer.remainingFocusTime
			s += fmt.Sprintf("%s", remainingFocusTime)
			s += fmt.Sprintf("\n%s\n\n", m.progress.View())
		} else {
			breakTime := m.timer.remainingBreakTime
			s += fmt.Sprintf("%s", breakTime)
			s += fmt.Sprintf("\n%s\n\n", m.progress.View())
		}

		if m.paused {
			s += "paused\n"
		}
	}

	if m.continueModal {
		if m.currentMode == 0 {
			s += headerStyle.Render("Proceed with the pomodoro") + "?\n\n"
		} else {
			s += headerStyle.Render("Proceed with the break") + "?\n\n"
		}

		for i, choice := range []string{"yes", "no"} {

			cursor := " "
			if m.cursor == i {
				cursor = ">"
			}

			s += fmt.Sprintf("%s %s\n", cursor, choiceStyle.Render(choice))
		}
	}

	s += "\n\n" + m.help.View(m.keys)

	return s
}

func main() {
	p := tea.NewProgram(initialModel())
	if _, err := p.Run(); err != nil {
		fmt.Printf("Alas, there's been an error: %v", err)
		os.Exit(1)
	}
}
