// WhatsApp for Claude (wa-approve.swift): ask the Mac's owner to approve a WhatsApp action with Touch ID.
// Installed under that name because macOS starts the prompt with it: "WhatsApp for Claude is trying to …".
// Biometrics only, no password field: a prompt that appears while the owner is
// typing elsewhere must not capture their keystrokes. The reason text is the only
// argument. Exit 0 approved, 1 declined or timed out, 2 Touch ID not available.
// ADR 2026-10-27-17d63a.
import Foundation
import LocalAuthentication

let reason = CommandLine.arguments.dropFirst().joined(separator: " ")
let context = LAContext()
context.localizedFallbackTitle = ""  // no "Enter Password" button
context.localizedCancelTitle = "Cancel"
var unavailable: NSError?
guard !reason.isEmpty, context.canEvaluatePolicy(.deviceOwnerAuthenticationWithBiometrics, error: &unavailable) else {
    FileHandle.standardError.write("wa-approve: \(unavailable?.localizedDescription ?? "no reason given")\n".data(using: .utf8)!)
    exit(2)
}
let done = DispatchSemaphore(value: 0)
var approved = false
context.evaluatePolicy(.deviceOwnerAuthenticationWithBiometrics, localizedReason: reason) { ok, _ in
    approved = ok
    done.signal()
}
if done.wait(timeout: .now() + 120) == .timedOut {
    context.invalidate()
    exit(1)
}
exit(approved ? 0 : 1)
