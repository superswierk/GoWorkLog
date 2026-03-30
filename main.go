package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"time"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var (
	docStyle      = lipgloss.NewStyle().Margin(1, 2)
	titleStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#00D7FF")).Bold(true)
	overtimeStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFD700")) // Złoty dla > 8h
	weekendStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#808080")) // Szary dla weekendów
	summaryStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#00FF00")).Bold(true).Padding(1)
)

type item struct {
	title, desc string
	month, year int
}

func (i item) Title() string       { return i.title }
func (i item) Description() string { return i.desc }
func (i item) FilterValue() string { return i.title }

type DayLog struct {
	Data    string  `json:"Data"`
	Start   string  `json:"Start"`
	Koniec  string  `json:"Koniec"`
	Godziny float64 `json:"Godziny"`
}

type model struct {
	list        list.Model
	logs        []DayLog
	loading     bool
	err         error
	viewingLogs bool
}

// BEZPIECZNE GENEROWANIE LISTY (bez duplikatów)
func getAvailableMonths() []list.Item {
	items := []list.Item{}
	now := time.Now()

	// Zaczynamy od 1. dnia obecnego miesiąca, aby AddDate działało przewidywalnie
	current := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.Local)

	for i := 0; i < 12; i++ {
		target := current.AddDate(0, -i, 0)
		monthName := target.Format("January")

		items = append(items, item{
			title: fmt.Sprintf("%s %d", monthName, target.Year()),
			desc:  fmt.Sprintf("Statystyki za %02d/%d", int(target.Month()), target.Year()),
			month: int(target.Month()),
			year:  target.Year(),
		})
	}
	return items
}

func fetchLogs(month, year int) tea.Cmd {
	return func() tea.Msg {
		cmd := exec.Command("powershell", "-ExecutionPolicy", "Bypass", "-File", "./get_logs.ps1",
			"-Month", strconv.Itoa(month), "-Year", strconv.Itoa(year))

		output, err := cmd.Output()
		if err != nil {
			return err
		}

		var logs []DayLog
		if err := json.Unmarshal(output, &logs); err != nil {
			return []DayLog{}
		}
		return logs
	}
}

func initialModel() model {
	l := list.New(getAvailableMonths(), list.NewDefaultDelegate(), 40, 15)
	l.Title = "HISTORIA CZASU PRACY"
	l.SetShowStatusBar(false)
	l.Styles.Title = titleStyle

	return model{list: l}
}

func (m model) Init() tea.Cmd { return nil }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if m.viewingLogs {
			if msg.String() == "esc" || msg.String() == "backspace" {
				m.viewingLogs = false
				return m, nil
			}
		}
		if msg.String() == "ctrl+c" || msg.String() == "q" {
			return m, tea.Quit
		}
		if msg.String() == "enter" && !m.viewingLogs {
			if i, ok := m.list.SelectedItem().(item); ok {
				m.loading = true
				m.viewingLogs = true
				return m, fetchLogs(i.month, i.year)
			}
		}
	case tea.WindowSizeMsg:
		h, v := docStyle.GetFrameSize()
		m.list.SetSize(msg.Width-h, msg.Height-v)
	case []DayLog:
		m.logs = msg
		m.loading = false
	}
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m model) View() string {
	if m.viewingLogs {
		if m.loading {
			return docStyle.Render("\n 🔍 Przeszukiwanie dziennika zdarzeń Windows...")
		}
		if len(m.logs) == 0 {
			return docStyle.Render("\n Brak zapisanych zdarzeń w tym miesiącu.\n\n [ESC] Powrót")
		}

		res := titleStyle.Render(fmt.Sprintf("RAPORT: %s", m.list.SelectedItem().(item).title)) + "\n\n"
		res += "DATA       | DZIEŃ | START    | KONIEC   | CZAS\n"
		res += "-----------|-------|----------|----------|---------\n"

		var total float64
		for _, l := range m.logs {
			t, _ := time.Parse("2006-01-02", l.Data)
			dayName := t.Format("Mon")
			isWeekend := t.Weekday() == time.Saturday || t.Weekday() == time.Sunday

			// Sprawdź czy to wpis "w toku"
			var isOngoing = false
			var displayKoniec = l.Koniec
			if len(l.Koniec) > 8 { // np. "14:20:01 (w toku)"
				isOngoing = true
			}

			line := fmt.Sprintf("%-10s | %-5s | %-8s | %-15s | ", l.Data, dayName, l.Start, displayKoniec)
			hoursStr := fmt.Sprintf("%.2f h", l.Godziny)

			// Logika kolorowania
			if isOngoing {
				// Niebieski kolor dla aktywnej sesji
				res += lipgloss.NewStyle().Foreground(lipgloss.Color("#5FAFFF")).Render(line + hoursStr + " 💻")
			} else if isWeekend {
				res += weekendStyle.Render(line + hoursStr)
			} else if l.Godziny > 8.0 {
				res += line + overtimeStyle.Render(hoursStr) + " 🔥"
			} else {
				res += line + hoursStr
			}
			res += "\n"
			total += l.Godziny
		}

		res += summaryStyle.Render(fmt.Sprintf("\nSUMA MIESIĘCZNA: %.2f h", total))
		res += "\n\n [ESC] Wróć do listy"
		return docStyle.Render(res)
	}
	return docStyle.Render(m.list.View())
}

func main() {
	if _, err := tea.NewProgram(initialModel(), tea.WithAltScreen()).Run(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
