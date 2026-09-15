import nltk
from nltk.corpus import wordnet as wn
from nltk.corpus import wordnet_ic

nltk.download('wordnet')
nltk.download('wordnet_ic')
brown_ic = wordnet_ic.ic('ic-brown.dat')

KEYWORDS = [
    "go", "golang", "frontend", "front end", "backend", "back end",
    "coding", "programming", "software engineering", "software",
    "typescript", "ts", "database", "orm", "orms", "sql", "sqlite",
    "postgres", "postgresql", "mysql", "linux",
]

ALIASES = {
    "go":                   "programming_language.n.01",
    "golang":               "programming_language.n.01",
    "typescript":           "programming_language.n.01",
    "ts":                   "programming_language.n.01",
    "coding":               "software.n.01",
    "programming":          "software.n.01",
    "frontend":             "software.n.01",
    "front end":            "software.n.01",
    "software engineering": "software.n.01",
    "software":             "software.n.01",
    "backend":              "software.n.01",
    "back end":             "software.n.01",
    "database":             "database.n.01",
    "orm":                  "database.n.01",
    "orms":                 "database.n.01",
    "sql":                  "database.n.01",
    "sqlite":               "database.n.01",
    "postgres":             "database.n.01",
    "postgresql":           "database.n.01",
    "mysql":                "database.n.01",
    "linux":                "operating_system.n.01",
}

MAINS = ["golang", "database", "typescript", "backend", "back end", "frontend", "front end", "linux"]

NON_MAIN_DECAY = 1

VALID = {s.name() for s in wn.all_synsets()}
for key in sorted(set(ALIASES.values())):
    if key not in VALID:
        raise SystemExit(f"Anchor '{key}' is not a valid WordNet synset. Fix ALIASES.")

def anchor(kw):
    return wn.synset(ALIASES[kw])

print("=== 1. COVERAGE ===")
for kw in KEYWORDS:
    senses = [s.name() for s in wn.synsets(kw)]
    flag = "" if senses else "   <-- not in WordNet (relying on ALIASES)"
    print(f"  {kw:22s} {senses}{flag}")

print("\n=== 2. WEIGHTS ===")
main_anchors = {m: anchor(m) for m in MAINS}

raw = {}
for kw in KEYWORDS:
    ka = anchor(kw)
    best = 0.0
    for m in MAINS:
        sim = ka.res_similarity(main_anchors[m], brown_ic)
        if sim is not None and sim > best:
            best = sim
    raw[kw] = best

max_raw = max(raw.values()) or 1.0
weights = {}
for kw in KEYWORDS:
    if kw in MAINS:
        weights[kw] = 1.0
    else:
        weights[kw] = (raw[kw] / max_raw) * NON_MAIN_DECAY

for kw in sorted(weights, key=lambda k: -weights[k]):
    bar = "#" * int(weights[kw] * 40)
    print(f"  {weights[kw]:6.3f}  {bar:40s} {kw}")

print("\n=== 3. SEED SQL ===")
for kw in sorted(weights, key=lambda k: -weights[k]):
    print(f"INSERT OR IGNORE INTO interests (keyword, weight, is_active) VALUES ('{kw}', {weights[kw]:.3f}, 1);")