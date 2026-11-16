package lumon

import (
	"strconv"
	"time"
)

// The plugin for coding agents opens the game while an agent works and sends
// an AgentMsg when it stops. The notice uses the escape notice's box.

const (
	agentTitle = "NOTICE"
	agentShort = "Your agent wants you"
)

// agentNotice is the sentence for msg, and for a finished turn, its summary
// on a line of its own.
func agentNotice(msg AgentMsg) string {
	if msg.NeedsApproval {
		return "Your agent in " + msg.Project + " needs your approval."
	}
	return "Your agent in " + msg.Project + " has finished its work." + agentSummary(msg)
}

// agentSummary is "\nIt took 4m 12s and 23 steps, and edited 5 files.", with
// whatever the agent could not report left out.
func agentSummary(msg AgentMsg) string {
	var took string
	switch {
	case msg.Took > 0 && msg.Steps > 0:
		took = spanText(msg.Took) + " and " + count(msg.Steps, "step")
	case msg.Took > 0:
		took = spanText(msg.Took)
	case msg.Steps > 0:
		took = count(msg.Steps, "step")
	default:
		return ""
	}
	s := "\nIt took " + took
	if msg.Edited > 0 {
		s += ", and edited " + count(msg.Edited, "file")
	}
	return s + "."
}

func count(n int, thing string) string {
	if n == 1 {
		return "1 " + thing
	}
	return strconv.Itoa(n) + " " + thing + "s"
}

// spanText is a duration as the strip and the notice write it: 42s, 4m 12s,
// 1h 5m.
func spanText(d time.Duration) string {
	d = d.Round(time.Second)
	h, m, s := int(d/time.Hour), int(d/time.Minute)%60, int(d/time.Second)%60
	switch {
	case h > 0:
		return strconv.Itoa(h) + "h " + strconv.Itoa(m) + "m"
	case m > 0 && s > 0:
		return strconv.Itoa(m) + "m " + strconv.Itoa(s) + "s"
	case m > 0:
		return strconv.Itoa(m) + "m"
	}
	return strconv.Itoa(max(s, 1)) + "s"
}
