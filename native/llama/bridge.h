// Thin C helpers around llama.h for the Go bridge. Everything that needs a C
// callback (progress, abort) or awkward struct handling lives here so the Go
// side only deals with plain functions.
#pragma once

#include <stdbool.h>
#include <stdint.h>
#include "llama.h"

typedef struct smol_ctx {
    struct llama_model   *model;
    struct llama_context *ctx;
    struct llama_sampler *smpl;
    const struct llama_vocab *vocab;
    volatile int abort;        // set by Go to interrupt llama_decode
} smol_ctx;

void smol_backend_init(void);

// Loads a model + context. progress_id identifies the Go-side progress
// listener (see goLoadProgress). Returns NULL and fills err on failure.
smol_ctx *smol_load(const char *path, int n_ctx, int n_threads, int n_batch,
                    uintptr_t progress_id, char *err, int err_len);
void smol_free(smol_ctx *s);

// (Re)configures the sampler chain.
void smol_set_sampler(smol_ctx *s, float temp, int top_k, float top_p,
                      float min_p, float repeat_penalty, int repeat_last_n,
                      uint32_t seed);

// Formats messages with the model's built-in chat template (falls back to
// ChatML, which every Smol model uses). Returns the byte length written, or
// the required length if buf is too small, or -1 on error.
int smol_apply_template(smol_ctx *s, const char **roles, const char **contents,
                        int n, char *buf, int buf_len);

int smol_tokenize(smol_ctx *s, const char *text, int text_len, int32_t *out,
                  int max, bool add_special);

// Removes cached KV entries from position p0 onward.
void smol_kv_truncate(smol_ctx *s, int p0);
void smol_kv_clear(smol_ctx *s);
// Highest cached position in sequence 0, or -1 when empty.
int  smol_kv_pos_max(smol_ctx *s);

// Decodes n tokens starting at the current cache position. Returns 0 on
// success, 2 if aborted, other values on error.
int smol_decode(smol_ctx *s, int32_t *tokens, int n);

int32_t smol_sample(smol_ctx *s);
bool    smol_is_eog(smol_ctx *s, int32_t tok);
int     smol_token_piece(smol_ctx *s, int32_t tok, char *buf, int len);
int     smol_n_ctx(smol_ctx *s);
