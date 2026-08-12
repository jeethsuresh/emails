/* email_core — C hot paths for MIME, search tokens, hashing, contacts.
 * Compiled to wasm32 and loaded by Electron main.
 */

typedef unsigned long size_t;

enum { HEAP_SIZE = 2 * 1024 * 1024 };
static unsigned char heap[HEAP_SIZE];
static size_t heap_off = 64;
static char outbuf[64 * 1024];

__attribute__((export_name("alloc")))
void *alloc(int n) {
  if (heap_off + (size_t)n >= HEAP_SIZE) return 0;
  void *p = &heap[heap_off];
  heap_off += (size_t)n;
  /* 8-byte align */
  heap_off = (heap_off + 7) & ~(size_t)7;
  return p;
}

__attribute__((export_name("dealloc")))
void dealloc(void *p, int n) {
  (void)p;
  (void)n;
  /* bump allocator; reset occasionally via export if needed */
}

static int ci_eq(char a, char b) {
  if (a >= 'A' && a <= 'Z') a = (char)(a - 'A' + 'a');
  if (b >= 'A' && b <= 'Z') b = (char)(b - 'A' + 'a');
  return a == b;
}

static int starts_header(const char *line, const char *name) {
  while (*name) {
    if (!*line || !ci_eq(*line, *name)) return 0;
    line++;
    name++;
  }
  return *line == ':';
}

__attribute__((export_name("mime_header_get")))
const char *mime_header_get(const char *raw, const char *name) {
  const char *p = raw;
  char *o = outbuf;
  *o = 0;
  while (*p) {
    const char *line = p;
    while (*p && *p != '\n') p++;
    if (starts_header(line, name)) {
      const char *v = line;
      while (*v && *v != ':') v++;
      if (*v == ':') v++;
      while (*v == ' ' || *v == '\t') v++;
      while (*v && *v != '\r' && *v != '\n' && (o - outbuf) < (int)sizeof(outbuf) - 1) {
        *o++ = *v++;
      }
      *o = 0;
      return outbuf;
    }
    if (*p == '\n') p++;
  }
  outbuf[0] = 0;
  return outbuf;
}

__attribute__((export_name("hash_blake_like")))
const char *hash_blake_like(const char *input) {
  /* FNV-1a 32-bit — stand-in until real BLAKE3 lands */
  unsigned int h = 2166136261u;
  for (const char *p = input; *p; p++) {
    h ^= (unsigned char)(*p);
    h *= 16777619u;
  }
  static const char *hex = "0123456789abcdef";
  for (int i = 7; i >= 0; i--) {
    outbuf[i] = hex[h & 0xf];
    h >>= 4;
  }
  outbuf[8] = 0;
  return outbuf;
}

static int is_tok(char c) {
  if (c >= 'a' && c <= 'z') return 1;
  if (c >= 'A' && c <= 'Z') return 1;
  if (c >= '0' && c <= '9') return 1;
  return c == '@' || c == '.' || c == '_' || c == '+' || c == '-';
}

__attribute__((export_name("index_tokenize")))
const char *index_tokenize(const char *text) {
  char *o = outbuf;
  const char *p = text;
  int first = 1;
  while (*p) {
    while (*p && !is_tok(*p)) p++;
    if (!*p) break;
    if (!first) {
      if ((o - outbuf) < (int)sizeof(outbuf) - 1) *o++ = '\n';
    }
    first = 0;
    while (*p && is_tok(*p) && (o - outbuf) < (int)sizeof(outbuf) - 1) {
      char c = *p++;
      if (c >= 'A' && c <= 'Z') c = (char)(c - 'A' + 'a');
      *o++ = c;
    }
  }
  *o = 0;
  return outbuf;
}

__attribute__((export_name("contact_normalize")))
const char *contact_normalize(const char *email) {
  char *o = outbuf;
  for (const char *p = email; *p && (o - outbuf) < (int)sizeof(outbuf) - 1; p++) {
    char c = *p;
    if (c == ' ' || c == '\t' || c == '\r' || c == '\n') continue;
    if (c >= 'A' && c <= 'Z') c = (char)(c - 'A' + 'a');
    *o++ = c;
  }
  *o = 0;
  return outbuf;
}
