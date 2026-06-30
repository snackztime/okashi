import Foundation
import FoundationModels

@available(macOS 26.0, *)
@Generable struct FMIssue {
    @Guide(description: "the exact wrong word or phrase, copied verbatim from the text")
    let wrong: String
    @Guide(description: "the corrected version of that word or phrase")
    let fix: String
    @Guide(description: "a short reason for the correction")
    let reason: String
}

@available(macOS 26.0, *)
@Generable struct FMResult {
    @Guide(description: "every grammar or spelling error found; empty if the text is correct")
    let issues: [FMIssue]
}

@_cdecl("okashi_fm_available")
public func okashi_fm_available() -> Int32 {
    if #available(macOS 26.0, *) {
        if case .available = SystemLanguageModel.default.availability { return 1 }
    }
    return 0
}

@_cdecl("okashi_fm_proofread")
public func okashi_fm_proofread(_ ctext: UnsafePointer<CChar>) -> UnsafeMutablePointer<CChar>? {
    let empty = "{\"issues\":[]}"
    let text = String(cString: ctext)
    guard #available(macOS 26.0, *) else { return strdup(empty) }
    let sem = DispatchSemaphore(value: 0)
    var json = empty
    Task {
        do {
            let s = LanguageModelSession(instructions: "You are a proofreader. Find grammar and spelling errors in the user's prose.")
            let r = try await s.respond(to: text, generating: FMResult.self)
            let issues = r.content.issues.map { ["wrong": $0.wrong, "fix": $0.fix, "reason": $0.reason] }
            let data = try JSONSerialization.data(withJSONObject: ["issues": issues])
            json = String(data: data, encoding: .utf8) ?? empty
        } catch {
            json = empty
        }
        sem.signal()
    }
    _ = sem.wait(timeout: .now() + 30) // bound a hung model; empty result on timeout
    return strdup(json)
}
