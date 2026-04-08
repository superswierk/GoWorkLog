package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

var (
	// Styl tła i marginesów
	mainStyle = lipgloss.NewStyle().Background(lipgloss.Color("#3f3d3d")).Padding(1, 2).MaxHeight(30)

	docStyle      = lipgloss.NewStyle().Margin(1, 2)
	titleStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#00D7FF")).Bold(true).Background(lipgloss.Color("#971919"))
	overtimeStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFD700")) // Złoty dla > 8h
	weekendStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#808080")) // Szary dla weekendów
	summaryStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#00FF00")).Bold(true).Padding(1)
	errorStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF0000")).Bold(true).Padding(0, 1).Background(lipgloss.Color("#3f3d3d"))
	headerStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#5F5FFF")).Bold(true).MarginBottom(1)
)

type item struct {
	title, desc string
	month, year int
	selected    bool
}

func (i item) Title() string {
	if i.selected {
		return "[x] " + i.title
	}
	return "[ ] " + i.title
}
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
	exportMsg   string
	isAdmin     bool
	width       int
	height      int
}

// Funkcja sprawdzająca uprawnienia administratora w Windows
func checkAdmin() bool {
	cmd := exec.Command("net", "session")
	err := cmd.Run()
	return err == nil
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
			title: lipgloss.Sprintf("%s %d", monthName, target.Year()),
			desc:  lipgloss.Sprintf("Statystyki za %02d/%d", int(target.Month()), target.Year()),
			month: int(target.Month()),
			year:  target.Year(),
		})
	}
	return items
}

func fetchLogsSync(month, year int) ([]DayLog, error) {
	psScript := lipgloss.Sprintf(`
$Month = %d
$Year = %d
$dzisiajDate = (Get-Date).Date
$teraz = Get-Date
$poczatekMiesiaca = Get-Date -Year $Year -Month $Month -Day 1 -Hour 0 -Minute 0 -Second 0
$koniecMiesiaca = $poczatekMiesiaca.AddMonths(1).AddSeconds(-1)
$eventIds = @(6005, 6006, 7001, 7002, 1, 42)
$lockIds = @(4800, 4801)

try {
    $events = @(Get-WinEvent -FilterHashtable @{
        LogName   = 'System'
        ID        = $eventIds
        StartTime = $poczatekMiesiaca
        EndTime   = $koniecMiesiaca
    } -ErrorAction SilentlyContinue)

    $lockEvents = @(Get-WinEvent -FilterHashtable @{
        LogName   = 'Security'
        ID        = $lockIds
        StartTime = $poczatekMiesiaca
        EndTime   = $koniecMiesiaca
    } -ErrorAction SilentlyContinue)

    # Poprawka nr 1: Nie przerywamy, jeśli rano nie wpadł log 'System', ale są logi 'Security'
    $allEvents = @($events + $lockEvents) | Where-Object { $_ -ne $null }
    if ($allEvents.Count -eq 0) { return "[]" }

    # Zbieramy wszystkie unikalne daty, z których mamy jakiekolwiek zdarzenia
    $unikalneDaty = $allEvents | ForEach-Object { $_.TimeCreated.Date } | Select-Object -Unique | Sort-Object

    $raport = $unikalneDaty | ForEach-Object {
        $dataZdarzenia = $_
        
        # Filtrujemy zdarzenia dla analizowanego dnia
        $zdarzeniaWdniu = @($events | Where-Object { $_ -ne $null -and $_.TimeCreated.Date -eq $dataZdarzenia } | Sort-Object TimeCreated)
        $blokiWdniu = @($lockEvents | Where-Object { $_ -ne $null -and $_.TimeCreated.Date -eq $dataZdarzenia } | Sort-Object TimeCreated)
        
        # Poprawka nr 2: Operujemy na całych obiektach zdarzeń, by mieć dostęp do .Id i .TimeCreated
        $pierwszeZdarzenie = $null
        $ostatnieZdarzenie = $null

        if ($zdarzeniaWdniu) { $pierwszeZdarzenie = $zdarzeniaWdniu[0] }
        elseif ($blokiWdniu) { $pierwszeZdarzenie = $blokiWdniu[0] }

        if ($zdarzeniaWdniu) { $ostatnieZdarzenie = $zdarzeniaWdniu[-1] }
        elseif ($blokiWdniu) { $ostatnieZdarzenie = $blokiWdniu[-1] }

        if ($pierwszeZdarzenie -eq $null) { return }

        $pierwsze = $pierwszeZdarzenie.TimeCreated
        $ostatnie = $ostatnieZdarzenie.TimeCreated

        # Sprawdzamy czy ostatni log sugeruje zamknięcie pracy (na oryginalnym obiekcie zdarzenia)
        $isClosing = $false
        if ($zdarzeniaWdniu) {
            $lastId = $zdarzeniaWdniu[-1].Id
            if ($lastId -eq 6006 -or $lastId -eq 42) { $isClosing = $true }
        }

        $status = ""
        $ostatnieWyswietlane = $ostatnie
        
        # Jeśli to dzisiaj i sesja nie ma logu zamknięcia, traktujemy ją jako "w toku"
        if ($dataZdarzenia -eq $dzisiajDate -and -not $isClosing) {
            $ostatnieWyswietlane = $teraz
            $status = " (w toku)"
        }
        
        $czasBrutto = $ostatnieWyswietlane - $pierwsze
        $czasBlokad = New-TimeSpan
        $lockStart = $null

        foreach ($ev in $blokiWdniu) {
            if ($ev.Id -eq 4800) { $lockStart = $ev.TimeCreated }
            elseif ($ev.Id -eq 4801 -and $lockStart -ne $null) {
                $czasBlokad += ($ev.TimeCreated - $lockStart)
                $lockStart = $null
            }
        }
        if ($lockStart -ne $null -and $dataZdarzenia -eq $dzisiajDate) { $czasBlokad += ($teraz - $lockStart) }

        $godzinyNetto = $czasBrutto.TotalHours - $czasBlokad.TotalHours
        if ($godzinyNetto -lt 0 -or $blokiWdniu.Count -eq 0) { $godzinyNetto = $czasBrutto.TotalHours }

        [PSCustomObject]@{
            Data    = $pierwsze.ToString("yyyy-MM-dd")
            Start   = $pierwsze.ToString("HH:mm:ss")
            Koniec  = $ostatnieWyswietlane.ToString("HH:mm:ss") + $status
            Godziny = [math]::Round($czasBrutto.TotalHours, 2)
            Netto   = [math]::Round($godzinyNetto, 2)
        }
    }
    $raport | ConvertTo-Json
} catch { "[]" }
`, month, year)

	cmd := exec.Command("powershell", "-ExecutionPolicy", "Bypass", "-Command", psScript)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, err
	}
	// DEBUG: Jeśli chcesz zobaczyć surowy wynik w konsoli podczas pracy, odkomentuj poniższą linię:
	//fmt.Printf("RAW PS OUTPUT: %s\n", string(output))

	var logs []DayLog
	if err := json.Unmarshal(output, &logs); err != nil {
		var singleLog DayLog
		if errSingle := json.Unmarshal(output, &singleLog); errSingle == nil {
			return []DayLog{singleLog}, nil
		}
		return nil, fmt.Errorf("JSON error: %v, Content: %s", err, string(output))
	}
	return logs, err
}

func fetchMultipleLogs(items []list.Item) tea.Cmd {
	return func() tea.Msg {
		var allLogs []DayLog
		anySelected := false
		for _, i := range items {
			it := i.(item)
			if it.selected {
				anySelected = true
				logs, _ := fetchLogsSync(it.month, it.year)
				allLogs = append(allLogs, logs...)
			}
		}

		if !anySelected {
			return []DayLog{}
		}

		sort.Slice(allLogs, func(i, j int) bool {
			return allLogs[i].Data < allLogs[j].Data
		})
		return allLogs
	}
}

func fetchLogs(month, year int) tea.Cmd {
	return func() tea.Msg {
		logs, err := fetchLogsSync(month, year)
		if err != nil {
			return []DayLog{}
		}
		return logs
	}
}

func exportSelected(items []list.Item) tea.Cmd {
	return func() tea.Msg {
		var allLogs []DayLog
		for _, i := range items {
			it := i.(item)
			if it.selected {
				logs, _ := fetchLogsSync(it.month, it.year)
				allLogs = append(allLogs, logs...)
			}
		}

		if len(allLogs) == 0 {
			return "Nie zaznaczono miesięcy do eksportu"
		}

		sort.Slice(allLogs, func(i, j int) bool {
			return allLogs[i].Data < allLogs[j].Data
		})

		f, err := os.Create("work_log_export.csv")
		if err != nil {
			return "Błąd tworzenia pliku"
		}
		defer f.Close()

		f.WriteString("Data;Dzien;Start;Koniec;Brutto;Netto\n")
		for _, l := range allLogs {
			t, _ := time.Parse("2006-01-02", l.Data)
			cleanKoniec := strings.ReplaceAll(l.Koniec, " (w toku)", "")
			line := lipgloss.Sprintf("%s;%s;%s;%s;%.2f;%.2f\n", l.Data, t.Format("Mon"), l.Start, cleanKoniec, l.Godziny, l.Netto)
			f.WriteString(strings.ReplaceAll(line, ".", ",")) // Zamiana kropki na przecinek dla Excela/Calc
		}

		return "Wyeksportowano do work_log_export.csv"
	}
}

func initialModel() model {
	d := list.NewDefaultDelegate()
	c := lipgloss.Color("#cddd72")
	d.Styles.NormalTitle.Background(c)
	d.Styles.NormalDesc.Background(c)
	d.Styles.SelectedTitle.Foreground(c)
	d.Styles.SelectedDesc.Background(c)
	l := list.New(getAvailableMonths(), d, 40, 15)
	l.Title = "WYBIERZ MIESIĄC"
	l.SetShowStatusBar(false)
	l.Styles.Title = titleStyle

	return model{
		list:    l,
		isAdmin: checkAdmin(),
	}
}

func (m model) Init() tea.Cmd { return nil }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case string:
		m.exportMsg = msg
		return m, nil
	case tea.KeyPressMsg:
		if m.viewingLogs {
			if msg.String() == "esc" || msg.String() == "backspace" || msg.String() == "q" {
				m.viewingLogs = false
				return m, nil
			}
		}
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
		if msg.String() == "q" && !m.viewingLogs {
			return m, tea.Quit
		}
		if msg.String() == "space" && !m.viewingLogs {
			idx := m.list.Index()
			it := m.list.Items()[idx].(item)
			it.selected = !it.selected
			m.list.SetItem(idx, it)
			return m, nil
		}
		if msg.String() == "x" && !m.viewingLogs {
			m.exportMsg = "Eksportowanie..."
			return m, exportSelected(m.list.Items())
		}
		if msg.String() == "enter" && !m.viewingLogs {
			hasSelection := false
			for _, i := range m.list.Items() {
				if i.(item).selected {
					hasSelection = true
					break
				}
			}

			m.loading = true
			m.viewingLogs = true
			m.exportMsg = ""

			if hasSelection {
				return m, fetchMultipleLogs(m.list.Items())
			} else {
				if i, ok := m.list.SelectedItem().(item); ok {
					return m, fetchLogs(i.month, i.year)
				}
			}
		}
	case tea.WindowSizeMsg:
		h, v := docStyle.GetFrameSize()
		m.list.SetSize(msg.Width-h, msg.Height-v)
		m.width, m.height = msg.Width, msg.Height
	case []DayLog:
		m.logs = msg
		m.loading = false
	}
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m model) View() tea.View {
	var v tea.View
	// Nagłówek ASCII
	header := headerStyle.Render(`GoWorkLog`)
	var adminWarning string
	if !m.isAdmin {
		adminWarning = errorStyle.Render("!!! BRAK UPRAWNIEŃ ADMINISTRATORA - CZAS NETTO BĘDZIE NIEDOKŁADNY !!!\n")
	}

	var content string
	if m.viewingLogs {
		if m.loading {
			content = docStyle.Render("\n 🔍 Przeszukiwanie dziennika zdarzeń Windows...")
		} else if len(m.logs) == 0 {
			content = docStyle.Render("\n Brak zapisanych zdarzeń.\n\n [ESC] Powrót")
		} else {
			res := titleStyle.Render("RAPORT SZCZEGÓŁOWY") + "\n\n"
			res += "DATA       | DZIEŃ | START    | KONIEC          | BRUTTO  | NETTO   \n"
			res += "-----------|-------|----------|-----------------|---------|---------\n"

			var total float64
			var totalNetto float64
			for _, l := range m.logs {
				t, _ := time.Parse("2006-01-02", l.Data)
				dayName := t.Format("Mon")
				isWeekend := t.Weekday() == time.Saturday || t.Weekday() == time.Sunday

				// Logika trwającej sesji
				isOngoing := strings.Contains(l.Koniec, "(w toku)")
				displayKoniec := l.Koniec

				line := lipgloss.Sprintf("%-10s | %-5s | %-8s | %-15s | %-7.2f | ", l.Data, dayName, l.Start, displayKoniec, l.Godziny)
				nettoStr := lipgloss.Sprintf("%.2f h", l.Netto)

				// Logika kolorowania
				if isOngoing {
					// Jasnoniebieski dla aktywnej sesji
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

			res += summaryStyle.Render(lipgloss.Sprintf("\nSUMA OKRESU BRUTTO: %.2f h | NETTO: %.2f h", total, totalNetto))
			res += "\n\n [ESC] Wróć do listy"
			content = docStyle.Render(res)
		}
	} else {
		help := "\n [SPACE] Zaznacz | [ENTER] Podgląd | [x] Eksportuj"
		content = docStyle.Background(lipgloss.Color("#297256")).Render(m.list.View() + help + "\n\n " + m.exportMsg)
	}

	// Renderowanie całości w kontenerze z tłem
	v.SetContent(mainStyle.
		Width(m.width).
		Height(m.height).
		Render(header + "\n" + adminWarning + "\n" + content))
	v.AltScreen = true
	return v
}

func main() {
	if _, err := tea.NewProgram(initialModel()).Run(); err != nil {
		lipgloss.Println(err)
		os.Exit(1)
	}
}
