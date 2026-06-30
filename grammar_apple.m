//go:build darwin && cgo && applegrammar

#import <AppKit/AppKit.h>
#include "grammar_apple.h"
#include <stdlib.h>
#include <string.h>

OkashiGrammarIssue *okashi_nsspell_check(const char *ctext, int *count) {
    *count = 0;
    @autoreleasepool {
        NSString *text = [NSString stringWithUTF8String:ctext];
        if (!text) return NULL;
        NSSpellChecker *checker = [NSSpellChecker sharedSpellChecker];
        NSInteger start = 0, len = (NSInteger)[text length];
        NSMutableArray<NSDictionary *> *found = [NSMutableArray array];
        while (start < len) {
            NSArray<NSDictionary *> *details = nil;
            NSRange r = [checker checkGrammarOfString:text
                                          startingAt:start
                                            language:@"en"
                                                wrap:NO
                              inSpellDocumentWithTag:0
                                             details:&details];
            if (r.location == NSNotFound) break;
            for (NSDictionary *d in details) {
                NSValue *rv = d[NSGrammarRange];
                NSRange gr = rv ? [rv rangeValue] : NSMakeRange(0, r.length);
                NSString *desc = d[NSGrammarUserDescription] ?: @"";
                [found addObject:@{@"loc": @(r.location + gr.location),
                                   @"len": @(gr.length),
                                   @"desc": desc}];
            }
            start = r.location + (r.length > 0 ? r.length : 1);
        }
        int n = (int)[found count];
        if (n == 0) return NULL;
        OkashiGrammarIssue *arr = calloc((size_t)n, sizeof(OkashiGrammarIssue));
        for (int i = 0; i < n; i++) {
            NSDictionary *d = found[(NSUInteger)i];
            arr[i].location = [d[@"loc"] longValue];
            arr[i].length = [d[@"len"] longValue];
            const char *u = [d[@"desc"] UTF8String];
            arr[i].desc = strdup(u ? u : "");
        }
        *count = n;
        return arr;
    }
}

void okashi_free_issues(OkashiGrammarIssue *issues, int count) {
    if (!issues) return;
    for (int i = 0; i < count; i++) free(issues[i].desc);
    free(issues);
}