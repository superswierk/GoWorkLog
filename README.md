# GoWorkLog

TUI application to track Windows work hours using System Event Logs.

## Features

* **Dual Time Tracking**: Brutto (total) vs Netto (active screen time).

* **Real-time**: Detects ongoing sessions (💻).

* **Visuals**: Overtime (🔥) and weekend highlighting.

## Enable Auditing (Required for Netto Time)
To track Win+L locks (Event 4800), you must enable the audit policy:

Press **Win + R**, type secpol.msc, press Enter.

Go to: *Advanced Audit Policy* -> *System Audit Policies* -> *Logon/Logoff*.

Enable **Audit Other Logon/Logoff Events** for **Success**.

## Build Instructions

```
go mod init GoWorkLog
go mod tidy
go build -o GoWorkLog.exe
```

## Usage

* **Arrows**: Navigate months.

* **Enter**: Fetch logs.

* **q / Esc**: Back to list or Exit.

## Requirements

Windows OS (PowerShell + Security Log access).

Go 1.18+ (for building).

## License
MIT