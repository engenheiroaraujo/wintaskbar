package desktop

import (
	"bytes"
	"encoding/xml"
	"path/filepath"
)

// StartupTaskXML monta a definição da tarefa agendada que inicia o exe ao
// fazer logon. A tarefa roda sem o atraso que o Windows aplica aos programas
// da chave Run, sem elevação e sem limite de tempo.
func StartupTaskXML(exe, user string) string {
	esc := func(s string) string {
		var b bytes.Buffer
		_ = xml.EscapeText(&b, []byte(s))
		return b.String()
	}
	return `<?xml version="1.0" encoding="UTF-16"?>
<Task version="1.2" xmlns="http://schemas.microsoft.com/windows/2004/02/mit/task">
  <RegistrationInfo><Description>wintaskbar</Description></RegistrationInfo>
  <Triggers><LogonTrigger><Enabled>true</Enabled><UserId>` + esc(user) + `</UserId></LogonTrigger></Triggers>
  <Principals><Principal id="Author"><UserId>` + esc(user) + `</UserId><LogonType>InteractiveToken</LogonType><RunLevel>LeastPrivilege</RunLevel></Principal></Principals>
  <Settings>
    <MultipleInstancesPolicy>IgnoreNew</MultipleInstancesPolicy>
    <DisallowStartIfOnBatteries>false</DisallowStartIfOnBatteries>
    <StopIfGoingOnBatteries>false</StopIfGoingOnBatteries>
    <ExecutionTimeLimit>PT0S</ExecutionTimeLimit>
    <Enabled>true</Enabled>
  </Settings>
  <Actions Context="Author"><Exec><Command>` + esc(exe) + `</Command><WorkingDirectory>` + esc(filepath.Dir(exe)) + `</WorkingDirectory></Exec></Actions>
</Task>`
}
