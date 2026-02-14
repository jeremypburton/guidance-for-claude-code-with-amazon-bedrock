package quota

import (
	"fmt"
	"os"
	"strings"
)

// HandleBlocked displays a user-friendly message when access is blocked by quota.
// Returns exit code 1.
func HandleBlocked(result *Result) int {
	message := result.Message
	if message == "" {
		message = "Access blocked due to quota limits"
	}

	stderr := os.Stderr
	fmt.Fprintln(stderr)
	fmt.Fprintln(stderr, strings.Repeat("=", 60))
	fmt.Fprintln(stderr, "ACCESS BLOCKED - QUOTA EXCEEDED")
	fmt.Fprintln(stderr, strings.Repeat("=", 60))
	fmt.Fprintf(stderr, "\n%s\n\n", message)

	if result.Usage != nil {
		fmt.Fprintln(stderr, "Current Usage:")
		if monthlyTokens, ok := result.Usage["monthly_tokens"]; ok {
			if monthlyLimit, ok2 := result.Usage["monthly_limit"]; ok2 {
				pct := getFloat(result.Usage, "monthly_percent")
				fmt.Fprintf(stderr, "  Monthly: %s / %s tokens (%.1f%%)\n",
					formatNumber(monthlyTokens), formatNumber(monthlyLimit), pct)
			}
		}
		if dailyTokens, ok := result.Usage["daily_tokens"]; ok {
			if dailyLimit, ok2 := result.Usage["daily_limit"]; ok2 {
				pct := getFloat(result.Usage, "daily_percent")
				fmt.Fprintf(stderr, "  Daily: %s / %s tokens (%.1f%%)\n",
					formatNumber(dailyTokens), formatNumber(dailyLimit), pct)
			}
		}
	}

	if result.Policy != nil {
		pType, _ := result.Policy["type"].(string)
		pID, _ := result.Policy["identifier"].(string)
		if pType != "" || pID != "" {
			fmt.Fprintf(stderr, "\nPolicy: %s:%s\n", pType, pID)
		}
	}

	fmt.Fprintln(stderr, "\nTo request an unblock, contact your administrator.")
	fmt.Fprintln(stderr, strings.Repeat("=", 60))
	fmt.Fprintln(stderr)

	return 1
}

// HandleWarning displays a quota warning without blocking access.
func HandleWarning(result *Result) {
	if result.Usage == nil {
		return
	}

	monthlyPercent := getFloat(result.Usage, "monthly_percent")
	dailyPercent := getFloat(result.Usage, "daily_percent")

	// Only show warning at 80%+
	if monthlyPercent < 80 && dailyPercent < 80 {
		return
	}

	stderr := os.Stderr
	fmt.Fprintln(stderr)
	fmt.Fprintln(stderr, strings.Repeat("=", 60))
	fmt.Fprintln(stderr, "QUOTA WARNING")
	fmt.Fprintln(stderr, strings.Repeat("=", 60))

	if monthlyTokens, ok := result.Usage["monthly_tokens"]; ok {
		if monthlyLimit, ok2 := result.Usage["monthly_limit"]; ok2 {
			fmt.Fprintf(stderr, "  Monthly: %s / %s tokens (%.1f%%)\n",
				formatNumber(monthlyTokens), formatNumber(monthlyLimit), monthlyPercent)
		}
	}
	if dailyTokens, ok := result.Usage["daily_tokens"]; ok {
		if dailyLimit, ok2 := result.Usage["daily_limit"]; ok2 {
			fmt.Fprintf(stderr, "  Daily: %s / %s tokens (%.1f%%)\n",
				formatNumber(dailyTokens), formatNumber(dailyLimit), dailyPercent)
		}
	}

	fmt.Fprintln(stderr, strings.Repeat("=", 60))
	fmt.Fprintln(stderr)
}

func getFloat(m map[string]interface{}, key string) float64 {
	v, ok := m[key]
	if !ok {
		return 0
	}
	switch n := v.(type) {
	case float64:
		return n
	case int:
		return float64(n)
	default:
		return 0
	}
}

func formatNumber(v interface{}) string {
	var n float64
	switch val := v.(type) {
	case float64:
		n = val
	case int:
		n = float64(val)
	default:
		return "0"
	}

	if n >= 1_000_000_000 {
		return fmt.Sprintf("%.1fB", n/1_000_000_000)
	} else if n >= 1_000_000 {
		return fmt.Sprintf("%.1fM", n/1_000_000)
	} else if n >= 1_000 {
		return fmt.Sprintf("%.1fK", n/1_000)
	}
	return fmt.Sprintf("%.0f", n)
}
