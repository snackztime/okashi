//go:build darwin && cgo && applegrammar

#ifndef GRAMMAR_APPLE_H
#define GRAMMAR_APPLE_H

// One NSSpellChecker grammar issue: UTF-16 offset+length into the checked string,
// plus a malloc'd UTF-8 description (caller frees via okashi_free_issues).
typedef struct {
    long location; // UTF-16 code-unit offset into the whole text
    long length;   // UTF-16 code-unit length
    char *desc;    // malloc'd UTF-8
} OkashiGrammarIssue;

// NSSpellChecker grammar check (AppKit). Returns a malloc'd array of *count issues,
// or NULL with *count=0 when there are none.
OkashiGrammarIssue *okashi_nsspell_check(const char *text, int *count);
void okashi_free_issues(OkashiGrammarIssue *issues, int count);

// Foundation Models (implemented in Swift). 1 if Apple Intelligence is available.
int okashi_fm_available(void);
// Proofread via the on-device model. Returns a malloc'd UTF-8 JSON string
// {"issues":[{"wrong":"..","fix":"..","reason":".."}]} (caller frees). NULL on error.
char *okashi_fm_proofread(const char *text);

#endif