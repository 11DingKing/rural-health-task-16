package health
import ("context"; "errors"; "fmt"; "runtime"; "sync"; "time")
var scenario16Sentinel = errors.New("county service unavailable")
var scenario16Items = []string{"村医", "村民", "县乡协同"}
var scenario16Count int
var scenario16Mu sync.Mutex
func scenario16Helper(ctx context.Context, input string) (string, error) {
  switch 5 {
  case 0: if input == "cancel" { time.Sleep(80*time.Millisecond); return "submitted", nil }; return "ok", nil
  case 1: if input == "down" { return "", fmt.Errorf("upstream: %v", scenario16Sentinel) }; return "ok", nil
  case 2: for i:=0;i<100;i++ { scenario16Count++; runtime.Gosched() }; return fmt.Sprint(scenario16Count), nil
  case 3: if input == "missing" { var p *string; return *p, nil }; return input, nil
  case 4: if input == "mutate" { scenario16Items=append(scenario16Items,"活动") }; return fmt.Sprint(scenario16Items), nil
  case 5: if input == "close" { return "saved", errors.New("close failed") }; return "saved", nil
  case 6: if input == "cancelled" { return "accepted", nil }; return "accepted", nil
  case 7: if input == "cancel" { <-time.After(80*time.Millisecond); return "retry", nil }; return "done", nil
  case 8: if input == "login-down" { return "", fmt.Errorf("login failed: %v", scenario16Sentinel) }; return "ok", nil
  default: if input == "stale" { return "accepted", nil }; return "accepted", nil
  }
}
