param (
    [int]$Month = (Get-Date).Month,
    [int]$Year = (Get-Date).Year
)

$dzisiajDate = (Get-Date).Date
$teraz = Get-Date
$poczatekMiesiaca = Get-Date -Year $Year -Month $Month -Day 1 -Hour 0 -Minute 0 -Second 0
$koniecMiesiaca = $poczatekMiesiaca.AddMonths(1).AddSeconds(-1)
$eventIds = @(6005, 6006, 7001, 7002, 1, 42)

try {
    $events = Get-WinEvent -FilterHashtable @{
        LogName   = 'System'
        ID        = $eventIds
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
        # Jeśli to dzisiaj i ostatnie zdarzenie to NIE jest wyłączenie (6006) ani uśpienie (42)
        if ($dataZdarzenia -eq $dzisiajDate -and $ostatnie.Id -notin @(6006, 42)) {
            $ostatnieWyswietlane = $teraz
            $status = " (w toku)"
        } else {
            $ostatnieWyswietlane = $ostatnie
            $status = ""
        }
        
        $czas = $ostatnieWyswietlane - $pierwsze

        [PSCustomObject]@{
            Data    = $pierwsze.ToString("yyyy-MM-dd")
            Start   = $pierwsze.ToString("HH:mm:ss")
            Koniec  = $ostatnieWyswietlane.ToString("HH:mm:ss") + $status
            Godziny = [math]::Round($czas.TotalHours, 2)
        }
    }
    $raport | ConvertTo-Json
} catch {
    "[]"
}