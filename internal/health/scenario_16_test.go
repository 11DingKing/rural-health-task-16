package health
import ("context"; "errors"; "strings"; "sync"; "testing"; "time")
func TestScenario16(t *testing.T) { ctx,cancel:=context.WithTimeout(context.Background(),10*time.Millisecond); defer cancel(); switch 5 {
case 0,7: start:=time.Now();_,err:=Scenario16(ctx,"cancel");if !errors.Is(err,context.DeadlineExceeded){t.Fatalf("expected deadline, got %v",err)};if time.Since(start)>60*time.Millisecond{t.Fatal("cancellation was not prompt")}
case 1,8: input:="down";if 5==8{input="login-down"};_,err:=Scenario16(context.Background(),input);if !errors.Is(err,scenario16Sentinel){t.Fatalf("error chain lost: %v",err)}
case 2: scenario16Count=0;var wg sync.WaitGroup;for i:=0;i<8;i++{wg.Add(1);go func(){defer wg.Done();_,_=Scenario16(context.Background(),"race")}()};wg.Wait();if scenario16Count!=800{t.Fatalf("count=%d",scenario16Count)}
case 3: defer func(){if r:=recover();r!=nil{t.Fatalf("missing profile panicked: %v",r)}}();if _,err:=Scenario16(context.Background(),"missing");err==nil{t.Fatal("missing profile should be rejected")}
case 4: got,_:=Scenario16(context.Background(),"mutate");if strings.Contains(got,"活动")||len(scenario16Items)!=3{t.Fatal("shared report state leaked")}
case 5:if _,err:=Scenario16(context.Background(),"close");err==nil{t.Fatal("close failure swallowed")}
case 6:if got,err:=Scenario16(context.Background(),"cancelled");err==nil&&got=="accepted"{t.Fatal("cancelled referral accepted")}
case 9:if got,err:=Scenario16(context.Background(),"stale");err==nil&&got=="accepted"{t.Fatal("stale version accepted")}
} }
