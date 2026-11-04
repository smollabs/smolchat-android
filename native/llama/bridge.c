#include "bridge.h"

#include <stdio.h>
#include <stdlib.h>
#include <string.h>

#ifdef __ANDROID__
#include <android/log.h>
#endif

// Implemented in Go (llama.go).
extern _Bool goLoadProgress(uintptr_t id, float progress);

static void smol_log(enum ggml_log_level level, const char *text, void *ud) {
    (void)ud;
    if (level < GGML_LOG_LEVEL_WARN) return; // keep logcat quiet
#ifdef __ANDROID__
    __android_log_print(level == GGML_LOG_LEVEL_ERROR ? ANDROID_LOG_ERROR : ANDROID_LOG_WARN,
                        "SmolChat/llama", "%s", text);
#else
    fputs(text, stderr);
#endif
}

void smol_backend_init(void) {
    llama_log_set(smol_log, NULL);
    llama_backend_init();
}

static bool progress_cb(float p, void *ud) {
    return goLoadProgress((uintptr_t)ud, p);
}

static bool abort_cb(void *ud) {
    return ((smol_ctx *)ud)->abort != 0;
}

smol_ctx *smol_load(const char *path, int n_ctx, int n_threads, int n_batch,
                    uintptr_t progress_id, char *err, int err_len) {
    struct llama_model_params mp = llama_model_default_params();
    mp.n_gpu_layers = 0;
    mp.load_mode = LLAMA_LOAD_MODE_MMAP; // weights stay file-backed where possible
    mp.progress_callback = progress_cb;
    mp.progress_callback_user_data = (void *)progress_id;

    struct llama_model *model = llama_model_load_from_file(path, mp);
    if (!model) {
        snprintf(err, err_len, "could not load model file (corrupt, unsupported or cancelled)");
        return NULL;
    }

    smol_ctx *s = calloc(1, sizeof(smol_ctx));
    s->model = model;
    s->vocab = llama_model_get_vocab(model);

    struct llama_context_params cp = llama_context_default_params();
    cp.n_ctx = n_ctx;
    cp.n_batch = n_batch;
    cp.n_ubatch = n_batch;
    cp.n_threads = n_threads;
    cp.n_threads_batch = n_threads;
    cp.no_perf = true;
    cp.abort_callback = abort_cb;
    cp.abort_callback_data = s;

    s->ctx = llama_init_from_model(model, cp);
    if (!s->ctx) {
        snprintf(err, err_len, "could not create inference context (not enough memory?)");
        llama_model_free(model);
        free(s);
        return NULL;
    }
    smol_set_sampler(s, 0.7f, 40, 0.95f, 0.05f, 1.1f, 64, LLAMA_DEFAULT_SEED);
    return s;
}

void smol_free(smol_ctx *s) {
    if (!s) return;
    if (s->smpl) llama_sampler_free(s->smpl);
    if (s->ctx) llama_free(s->ctx);
    if (s->model) llama_model_free(s->model);
    free(s);
}

void smol_set_sampler(smol_ctx *s, float temp, int top_k, float top_p,
                      float min_p, float repeat_penalty, int repeat_last_n,
                      uint32_t seed) {
    if (s->smpl) llama_sampler_free(s->smpl);
    struct llama_sampler_chain_params sp = llama_sampler_chain_default_params();
    sp.no_perf = true;
    struct llama_sampler *c = llama_sampler_chain_init(sp);
    if (repeat_penalty > 1.0f)
        llama_sampler_chain_add(c, llama_sampler_init_penalties(llama_vocab_n_tokens(s->vocab), repeat_last_n, repeat_penalty, 0.0f, 0.0f));
    if (temp <= 0.0f) {
        llama_sampler_chain_add(c, llama_sampler_init_greedy());
    } else {
        llama_sampler_chain_add(c, llama_sampler_init_top_k(top_k));
        llama_sampler_chain_add(c, llama_sampler_init_top_p(top_p, 1));
        llama_sampler_chain_add(c, llama_sampler_init_min_p(min_p, 1));
        llama_sampler_chain_add(c, llama_sampler_init_temp(temp));
        llama_sampler_chain_add(c, llama_sampler_init_dist(seed));
    }
    s->smpl = c;
}

int smol_apply_template(smol_ctx *s, const char **roles, const char **contents,
                        int n, char *buf, int buf_len) {
    struct llama_chat_message *msgs = calloc(n, sizeof(struct llama_chat_message));
    for (int i = 0; i < n; i++) {
        msgs[i].role = roles[i];
        msgs[i].content = contents[i];
    }
    const char *tmpl = llama_model_chat_template(s->model, NULL);
    int r = llama_chat_apply_template(tmpl, msgs, n, true, buf, buf_len);
    if (r < 0) // template not recognised: every Smol model is ChatML-based
        r = llama_chat_apply_template("chatml", msgs, n, true, buf, buf_len);
    free(msgs);
    return r;
}

int smol_tokenize(smol_ctx *s, const char *text, int text_len, int32_t *out,
                  int max, bool add_special) {
    return llama_tokenize(s->vocab, text, text_len, out, max, add_special, true);
}

void smol_kv_truncate(smol_ctx *s, int p0) {
    llama_memory_seq_rm(llama_get_memory(s->ctx), 0, p0, -1);
}

void smol_kv_clear(smol_ctx *s) {
    llama_memory_clear(llama_get_memory(s->ctx), true);
}

int smol_kv_pos_max(smol_ctx *s) {
    return (int)llama_memory_seq_pos_max(llama_get_memory(s->ctx), 0);
}

int smol_decode(smol_ctx *s, int32_t *tokens, int n) {
    struct llama_batch b = llama_batch_get_one(tokens, n);
    return llama_decode(s->ctx, b);
}

int32_t smol_sample(smol_ctx *s) {
    return llama_sampler_sample(s->smpl, s->ctx, -1);
}

bool smol_is_eog(smol_ctx *s, int32_t tok) {
    return llama_vocab_is_eog(s->vocab, tok);
}

int smol_token_piece(smol_ctx *s, int32_t tok, char *buf, int len) {
    return llama_token_to_piece(s->vocab, tok, buf, len, 0, false);
}

int smol_n_ctx(smol_ctx *s) {
    return (int)llama_n_ctx(s->ctx);
}
