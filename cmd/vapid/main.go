package main

import (
  "fmt"
  "log"
  webpush "github.com/SherClockHolmes/webpush-go"
)
func main() {
  private,public,err:=webpush.GenerateVAPIDKeys()
  if err!=nil {log.Fatal(err)}
  fmt.Printf("# Keep VAPID_PRIVATE_KEY secret. Reuse keys across server restarts.\nVAPID_PUBLIC_KEY=%s\nVAPID_PRIVATE_KEY=%s\nVAPID_SUBJECT=mailto:you@example.com\n",public,private)
}
