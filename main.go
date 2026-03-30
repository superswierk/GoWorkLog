package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
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
	Netto   float64 `json:"Netto"`
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

	// Zaczynamy od 1. dnia obecnego miasta, aby AddDate działało przewidywalnie
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
		// Cały skrypt PowerShell osadzony bezpośrednio w kodzie Go
		psScript := fmt.Sprintf(`
$Month = %d
$Year = %d
$dzisiajDate = (Get-Date).Date
$teraz = Get-Date
$poczatekMiesiaca = Get-Date -Year $Year -Month $Month -Day 1 -Hour 0 -Minute 0 -Second 0
$koniecMiesiaca = $poczatekMiesiaca.AddMonths(1).AddSeconds(-1)

# Główne eventy systemowe (Power/Session)
$eventIds = @(6005, 6006, 7001, 7002, 1, 42)
# Eventy blokady (Security Log)
$lockIds = @(4800, 4801)

try {
    $events = Get-WinEvent -FilterHashtable @{
        LogName   = 'System'
        ID        = $eventIds
        StartTime = $poczatekMiesiaca
        EndTime   = $koniecMiesiaca
    } -ErrorAction SilentlyContinue

    # Próba pobrania eventów blokady (wymaga uprawnień i włączonego audytu)
    $lockEvents = Get-WinEvent -FilterHashtable @{
        LogName   = 'Security'
        ID        = $lockIds
        StartTime = $poczatekMiesiaca
        EndTime   = $koniecMiesiaca
    } -ErrorAction SilentlyContinue

    if (-not $events) { return "[]" }

    $raport = $events | Group-Object { $_.TimeCreated.Date } | Sort-Object { [datetime]$_.Name } | ForEach-Object {
        $dataZdarzenia = [datetime]$_.Name
        $zdarzeniaWdniu = $_.Group | Sort-Object TimeCreated
        
        $pierwsze = $zdarzeniaWdniu[0].TimeCreated
        $ostatnie = $zdarzeniaWdniu[-1].TimeCreated
        
        # LOGIKA DLA DNIA DZISIEJSZEGO:
        if ($dataZdarzenia -eq $dzisiajDate -and $ostatnie.Id -notin @(6006, 42)) {
            $ostatnieWyswietlane = $teraz
            $status = " (w toku)"
        } else {
            $ostatnieWyswietlane = $ostatnie
            $status = ""
        }
        
        # Obliczanie czasu BRUTTO
        $czasBrutto = $ostatnieWyswietlane - $pierwsze
        
        # Obliczanie czasu NETTO (odejmowanie blokad ekranu)
        $blokiWdniu = $lockEvents | Where-Object { $_.TimeCreated.Date -eq $dataZdarzenia } | Sort-Object TimeCreated
        $czasBlokad = New-TimeSpan
        $lockStart = $null

        foreach ($ev in $blokiWdniu) {
            if ($ev.Id -eq 4800) { # Zablokowano
                $lockStart = $ev.TimeCreated
            } elseif ($ev.Id -eq 4801 -and $lockStart -ne $null) { # Odblokowano
                $czasBlokad += ($ev.TimeCreated - $lockStart)
                $lockStart = $null
            }
        }
        # Jeśli ostatnia blokada wciąż trwa (np. dzisiaj)
        if ($lockStart -ne $null -and $dataZdarzenia -eq $dzisiajDate) {
            $czasBlokad += ($teraz - $lockStart)
        }

        $godzinyNetto = $czasBrutto.TotalHours - $czasBlokad.TotalHours
        if ($godzinyNetto -lt 0) { $godzinyNetto = $czasBrutto.TotalHours }

        [PSCustomObject]@{
            Data    = $pierwsze.ToString("yyyy-MM-dd")
            Start   = $pierwsze.ToString("HH:mm:ss")
            Koniec  = $ostatnieWyswietlane.ToString("HH:mm:ss") + $status
            Godziny = [math]::Round($czasBrutto.TotalHours, 2)
            Netto   = [math]::Round($godzinyNetto, 2)
        }
    }
    $raport | ConvertTo-Json
} catch {
    "[]"
}
`, month, year)

		cmd := exec.Command("powershell", "-ExecutionPolicy", "Bypass", "-Command", psScript)

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
			if msg.String() == "esc" || msg.String() == "backspace" || msg.String() == "q" {
				m.viewingLogs = false
				return m, nil
			}
		}
		if msg.String() == "ctrl+c" || (msg.String() == "q" && !m.viewingLogs) {
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
		res += "DATA       | DZIEŃ | START    | KONIEC          | BRUTTO  | NETTO   \n"
		res += "-----------|-------|----------|-----------------|---------|---------\n"

		var total float64
		var totalNetto float64
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

			line := fmt.Sprintf("%-10s | %-5s | %-8s | %-15s | %-7.2f | ", l.Data, dayName, l.Start, displayKoniec, l.Godziny)
			nettoStr := fmt.Sprintf("%.2f h", l.Netto)

			// Logika kolorowania
			if isOngoing {
				// Niebieski kolor dla aktywnej sesji
				res += lipgloss.NewStyle().Foreground(lipgloss.Color("#5FAFFF")).Render(line + nettoStr + " 💻")
			} else if isWeekend {
				res += weekendStyle.Render(line + nettoStr)
			} else if l.Netto > 8.0 {
				res += line + overtimeStyle.Render(nettoStr) + " 🔥"
			} else {
				res += line + nettoStr
			}
			res += "\n"
			total += l.Godziny
			totalNetto += l.Netto
		}

		res += summaryStyle.Render(fmt.Sprintf("\nSUMA MIESIĘCZNA BRUTTO: %.2f h | NETTO: %.2f h", total, totalNetto))
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
