# GoWorkLog

TUI application to track Windows work hours using System Event Logs.

## Features

* **Dual Time Tracking**: Brutto (total) vs Netto (active screen time).

* **Multi-Month Export**: Select months and export to CSV.

* **Real-time**: Detects ongoing sessions.

## Enable Auditing (Required for Netto Time)
To track Win+L locks (Event 4800), you must enable the audit policy:

Press **Win + R**, type secpol.msc, press Enter.

Go to: *Advanced Audit Policy* -> *System Audit Policies* -> *Logon/Logoff*.

Enable **Audit Other Logon/Logoff Events** for **Success**.

## Build Instructions

```
go mod init GoWorkLog
go mod tidy
go build -o GoWorkLog.exe main.go
```

## Usage

* **Arrows**: Navigate months.

* **Space**: Select/deselect months for export.

* **x**: Export selected months to work_log_export.csv.

* **Enter**: View detailed logs for the selected month.

* **q / Esc**: Back to list or Exit.

## Requirements

Windows OS (PowerShell + Security Log access).

Go 1.18+ (for building).

## License
MIT