Output "assets/kube-prompt-demo.gif"

Require kube-prompt
Require kubectl

Set FontSize 18
Set Width 1100
Set Height 680
Set TypingSpeed 35ms

Type "kube-prompt --kubeconfig /demo/kubeconfig --default-namespace apps"
Enter
Sleep 2s

Type "/namespace"
Enter
Sleep 1s

Type "get pods | grep web"
Enter
Sleep 2s

Ctrl+S
Sleep 900ms
Type "q"
Sleep 500ms

Type "get pods deployment/web"
Enter
Sleep 2s

Type "get widgets alpha"
Enter
Sleep 2s

Type "/outputs"
Enter
Sleep 900ms
Type "q"
Sleep 500ms

Type "/output save last /tmp/kube-prompt-demo-output.log"
Enter
Sleep 1s

Type "/exit"
Enter
Sleep 500ms
