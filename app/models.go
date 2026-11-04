package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// Category groups models in the library.
type Category string

const (
	CategoryText      Category = "text"
	CategoryVision    Category = "vision"
	CategoryDocument  Category = "document"
	CategoryEmbedding Category = "embedding"
)

// Categories in display order.
var Categories = []Category{CategoryText, CategoryVision, CategoryDocument, CategoryEmbedding}

func (c Category) Title() string {
	switch c {
	case CategoryVision:
		return "Vision"
	case CategoryDocument:
		return "Documents"
	case CategoryEmbedding:
		return "Embeddings"
	}
	return "Text"
}

// Backend names the inference adapter a model needs.
type Backend string

const (
	BackendLlama     Backend = "llama.cpp"      // text GGUF via llama.cpp (implemented)
	BackendLlamaMTMD Backend = "llama.cpp-mtmd" // GGUF + mmproj via libmtmd (not yet wired)
	BackendNone      Backend = "none"           // no on-device GGUF runtime exists
)

// ModelFile is one file of a model repository.
type ModelFile struct {
	Name   string `json:"name"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"` // LFS oid from the Hub
	Role   string `json:"role"`   // "weights" or "mmproj"
}

// Arch carries the dimensions needed to estimate KV-cache memory.
type Arch struct {
	Layers   int
	KVHeads  int
	HeadDim  int
	TrainCtx int
}

// ModelSpec is a catalog entry. All repository ids, revisions, file names,
// sizes and hashes were verified against the Hugging Face Hub API.
type ModelSpec struct {
	ID           string
	Name         string
	Family       string
	Params       string
	ParamsM      int // parameter count in millions, used for routing
	Category     Category
	Backend      Backend
	Repo         string
	Revision     string // pinned commit
	Publisher    string // who published the GGUF conversion
	License      string
	Quant        string
	Description  string
	Capabilities []string
	Files        []ModelFile
	Arch         Arch
	// Supported is true when SmolChat has a working inference workflow.
	Supported   bool
	Unsupported string // why not, when !Supported
	// Advanced marks large downloads the UI de-emphasises.
	Advanced bool
}

// DownloadSize is the sum of all file sizes.
func (m ModelSpec) DownloadSize() int64 {
	var n int64
	for _, f := range m.Files {
		n += f.Size
	}
	return n
}

// Weights returns the main GGUF file.
func (m ModelSpec) Weights() ModelFile {
	for _, f := range m.Files {
		if f.Role == "weights" {
			return f
		}
	}
	return ModelFile{}
}

// FileURL is the Hub download URL of f at the pinned revision.
func (m ModelSpec) FileURL(f ModelFile) string {
	return fmt.Sprintf("https://huggingface.co/%s/resolve/%s/%s", m.Repo, m.Revision, f.Name)
}

// RepoURL links to the model page.
func (m ModelSpec) RepoURL() string { return "https://huggingface.co/" + m.Repo }

// EstimateRuntimeBytes estimates resident memory while running with a context
// of nCtx tokens: weights + f16 KV cache + compute buffers + fixed overhead.
func (m ModelSpec) EstimateRuntimeBytes(nCtx int) int64 {
	w := m.Weights().Size
	kv := int64(2*m.Arch.Layers*m.Arch.KVHeads*m.Arch.HeadDim*2) * int64(nCtx)
	compute := int64(64<<20) + w/12
	const overhead = 120 << 20 // Go heap, Gio, GPU surfaces
	return w + kv + compute + overhead
}

var smolLM1 = "Original SmolLM instruct model (v0.2). Superseded by SmolLM2 but still a capable tiny assistant."

// Catalog is the built-in, verified model registry. It ships with the app so
// the library works offline; RefreshRegistry can re-verify it online.
var Catalog = []ModelSpec{
	{
		ID: "smollm2-135m-instruct-q8", Name: "SmolLM2 135M Instruct", Family: "SmolLM2", Params: "135M", ParamsM: 135,
		Category: CategoryText, Backend: BackendLlama, Supported: true,
		Repo: "bartowski/SmolLM2-135M-Instruct-GGUF", Revision: "09816acd5d99df7be770d85ea30822623dab342c",
		Publisher: "bartowski (community GGUF of HuggingFaceTB/SmolLM2-135M-Instruct; no official GGUF exists)", License: "Apache-2.0",
		Quant:        "Q8_0",
		Description:  "The tiniest SmolLM2. Instant replies for quick questions, rewriting and small talk on any phone.",
		Capabilities: []string{"Chat", "Quick answers", "Rewriting"},
		Files:        []ModelFile{{"SmolLM2-135M-Instruct-Q8_0.gguf", 144811360, "5a1395716f7913741cc51d98581b9b1228d80987a9f7d3664106742eb06bba83", "weights"}},
		Arch:         Arch{30, 3, 64, 8192},
	},
	{
		ID: "smollm2-360m-instruct-q8", Name: "SmolLM2 360M Instruct", Family: "SmolLM2", Params: "360M", ParamsM: 360,
		Category: CategoryText, Backend: BackendLlama, Supported: true,
		Repo: "HuggingFaceTB/SmolLM2-360M-Instruct-GGUF", Revision: "593b5a2e04c8f3e4ee880263f93e0bd2901ad47f",
		Publisher: "HuggingFaceTB (official)", License: "Apache-2.0",
		Quant:        "Q8_0",
		Description:  "The best balance of speed and quality for most phones. Recommended first download.",
		Capabilities: []string{"Chat", "Summaries", "Explanations"},
		Files:        []ModelFile{{"smollm2-360m-instruct-q8_0.gguf", 386404992, "48ab3034d0dd401fbc721eb1df3217902fee7dab9078992d66431f09b7750201", "weights"}},
		Arch:         Arch{32, 5, 64, 8192},
	},
	{
		ID: "smollm2-1.7b-instruct-q4km", Name: "SmolLM2 1.7B Instruct", Family: "SmolLM2", Params: "1.7B", ParamsM: 1700,
		Category: CategoryText, Backend: BackendLlama, Supported: true,
		Repo: "HuggingFaceTB/SmolLM2-1.7B-Instruct-GGUF", Revision: "2d4a76a30b4af41ecd395c35725ac11688d4cfe4",
		Publisher: "HuggingFaceTB (official)", License: "Apache-2.0",
		Quant:        "Q4_K_M",
		Description:  "The most capable SmolLM2. Better reasoning, writing and coding; needs a phone with 4 GB+ RAM.",
		Capabilities: []string{"Chat", "Reasoning", "Writing", "Code"},
		Files:        []ModelFile{{"smollm2-1.7b-instruct-q4_k_m.gguf", 1055609536, "decd2598bc2c8ed08c19adc3c8fdd461ee19ed5708679d1c54ef54a5a30d4f33", "weights"}},
		Arch:         Arch{24, 32, 64, 8192},
	},
	{
		ID: "smollm3-3b-q4km", Name: "SmolLM3 3B", Family: "SmolLM3", Params: "3B", ParamsM: 3000,
		Category: CategoryText, Backend: BackendLlama, Supported: true, Advanced: true,
		Repo: "ggml-org/SmolLM3-3B-GGUF", Revision: "4965cb60b150737b68a0408c36aeefb65078f894",
		Publisher: "ggml-org (llama.cpp maintainers)", License: "Apache-2.0",
		Quant:        "Q4_K_M",
		Description:  "SmolLM3 with long context and step-by-step reasoning. Advanced: large download, needs 6 GB+ RAM and runs slower.",
		Capabilities: []string{"Chat", "Reasoning", "Multilingual", "Code"},
		Files:        []ModelFile{{"SmolLM3-Q4_K_M.gguf", 1915305312, "8334b850b7bd46238c16b0c550df2138f0889bf433809008cc17a8b05761863e", "weights"}},
		Arch:         Arch{36, 4, 128, 65536},
	},
	{
		ID: "smollm-135m-instruct-q8", Name: "SmolLM 135M Instruct", Family: "SmolLM", Params: "135M", ParamsM: 135,
		Category: CategoryText, Backend: BackendLlama, Supported: true,
		Repo: "HuggingFaceTB/smollm-135M-instruct-v0.2-Q8_0-GGUF", Revision: "53066c72a815622e28d006aefbe32df6dd4b1835",
		Publisher: "HuggingFaceTB (official)", License: "Apache-2.0",
		Quant: "Q8_0", Description: smolLM1, Capabilities: []string{"Chat"},
		Files: []ModelFile{{"smollm-135m-instruct-add-basics-q8_0.gguf", 144811552, "a98d3857b95b96c156d954780d28f39dcb35b642e72892ee08ddff70719e6220", "weights"}},
		Arch:  Arch{30, 3, 64, 2048},
	},
	{
		ID: "smollm-360m-instruct-q8", Name: "SmolLM 360M Instruct", Family: "SmolLM", Params: "360M", ParamsM: 360,
		Category: CategoryText, Backend: BackendLlama, Supported: true,
		Repo: "HuggingFaceTB/smollm-360M-instruct-v0.2-Q8_0-GGUF", Revision: "ac681f051bd393dc26c5bdacd277c03a7ca5d5d2",
		Publisher: "HuggingFaceTB (official)", License: "Apache-2.0",
		Quant: "Q8_0", Description: smolLM1, Capabilities: []string{"Chat"},
		Files: []ModelFile{{"smollm-360m-instruct-add-basics-q8_0.gguf", 386405440, "b5a2e94a0be8c047bccc3f52bc2f27b79b0dbc688ef3102de54fa6a33b20198c", "weights"}},
		Arch:  Arch{32, 5, 64, 2048},
	},
	{
		ID: "smollm-1.7b-instruct-q8", Name: "SmolLM 1.7B Instruct", Family: "SmolLM", Params: "1.7B", ParamsM: 1700,
		Category: CategoryText, Backend: BackendLlama, Supported: true, Advanced: true,
		Repo: "HuggingFaceTB/smollm-1.7B-instruct-v0.2-Q8_0-GGUF", Revision: "21c643bffffcc401ae24caab373b417de27949a5",
		Publisher: "HuggingFaceTB (official)", License: "Apache-2.0",
		Quant: "Q8_0", Description: smolLM1 + " Only an 8-bit build is published, so it is large.", Capabilities: []string{"Chat", "Writing"},
		Files: []ModelFile{{"smollm-1.7b-instruct-add-basics-q8_0.gguf", 1820415104, "3dfb6e468f9a8d6746bc9b4241eca84abd3b1acb851420431ee8db870c784f79", "weights"}},
		Arch:  Arch{24, 32, 64, 2048},
	},

	// Vision: GGUF + mmproj exist (ggml-org), but SmolChat does not yet ship
	// libmtmd or an image picker, so these are listed but not downloadable.
	vlm("smolvlm-256m-instruct-q8", "SmolVLM 256M Instruct", "SmolVLM", "256M", 256, "ggml-org/SmolVLM-256M-Instruct-GGUF", "b9e4379657e1450d04d02eec8e345667265b0a00", "Q8_0",
		ModelFile{"SmolVLM-256M-Instruct-Q8_0.gguf", 175054528, "2a31195d3769c0b0fd0a4906201666108834848db768af11de1d2cef7cd35e65", "weights"},
		ModelFile{"mmproj-SmolVLM-256M-Instruct-Q8_0.gguf", 103769856, "7e943f7c53f0382a6fc41b6ee0c2def63ba4fded9ab8ed039cc9e2ab905e0edd", "mmproj"}),
	vlm("smolvlm-500m-instruct-q8", "SmolVLM 500M Instruct", "SmolVLM", "500M", 500, "ggml-org/SmolVLM-500M-Instruct-GGUF", "72e986006ef53e37cdd3f6d4241c90b0f01df376", "Q8_0",
		ModelFile{"SmolVLM-500M-Instruct-Q8_0.gguf", 436806912, "9d4612de6a42214499e301494a3ecc2be0abdd9de44e663bda63f1152fad1bf4", "weights"},
		ModelFile{"mmproj-SmolVLM-500M-Instruct-Q8_0.gguf", 108783360, "d1eb8b6b23979205fdf63703ed10f788131a3f812c7b1f72e0119d5d81295150", "mmproj"}),
	vlm("smolvlm-2.2b-instruct-q4km", "SmolVLM 2.2B Instruct", "SmolVLM", "2.2B", 2200, "ggml-org/SmolVLM-Instruct-GGUF", "e75618fdae83145c487f1b4d8115bb02a5b58ecc", "Q4_K_M",
		ModelFile{"SmolVLM-Instruct-Q4_K_M.gguf", 1112242368, "dc80966bd84789de64115f07888939c03abb1714d431c477dfb405517a554af5", "weights"},
		ModelFile{"mmproj-SmolVLM-Instruct-Q8_0.gguf", 592521344, "86b84aa7babf1ab51a6366d973b9d380354e92c105afaa4f172cc76d044da739", "mmproj"}),
	vlm("smolvlm2-256m-video-instruct-q8", "SmolVLM2 256M Video Instruct", "SmolVLM2", "256M", 256, "ggml-org/SmolVLM2-256M-Video-Instruct-GGUF", "a8383113a70b406f088ac936e5f1918ed08e620d", "Q8_0",
		ModelFile{"SmolVLM2-256M-Video-Instruct-Q8_0.gguf", 175056256, "1202d1c54493bddff5b0ecbc36fcb7520ff720b9fa3d7224aeb293581f90529a", "weights"},
		ModelFile{"mmproj-SmolVLM2-256M-Video-Instruct-Q8_0.gguf", 103771616, "05d5751132244a6ebd64cba9b34898c0d874b2cb78159d758e1d4da3aad91581", "mmproj"}),
	vlm("smolvlm2-500m-video-instruct-q8", "SmolVLM2 500M Video Instruct", "SmolVLM2", "500M", 500, "ggml-org/SmolVLM2-500M-Video-Instruct-GGUF", "ccd7aae53bcb1997355c2f094959e72b3642ce17", "Q8_0",
		ModelFile{"SmolVLM2-500M-Video-Instruct-Q8_0.gguf", 436808704, "6f67b8036b2469fcd71728702720c6b51aebd759b78137a8120733b4d66438bc", "weights"},
		ModelFile{"mmproj-SmolVLM2-500M-Video-Instruct-Q8_0.gguf", 108785184, "921dc7e259f308e5b027111fa185efcbf33db13f6e35749ddf7f5cdb60ef520b", "mmproj"}),
	vlm("smolvlm2-2.2b-instruct-q4km", "SmolVLM2 2.2B Instruct", "SmolVLM2", "2.2B", 2200, "ggml-org/SmolVLM2-2.2B-Instruct-GGUF", "1bc3c9f74ceafd4c8d4411cc9cf188bba3798f91", "Q4_K_M",
		ModelFile{"SmolVLM2-2.2B-Instruct-Q4_K_M.gguf", 1112602656, "0cf76814555b8665149075b74ab6b5c1d428ea1d3d01c1918c12012e8d7c9f58", "weights"},
		ModelFile{"mmproj-SmolVLM2-2.2B-Instruct-Q8_0.gguf", 592523200, "ae07ea1facd07dd3230c4483b63e8cda96c6944ad2481f33d531f79e892dd024", "mmproj"}),

	{
		ID: "smoldocling-256m-preview", Name: "SmolDocling 256M Preview", Family: "SmolDocling", Params: "256M", ParamsM: 256,
		Category: CategoryDocument, Backend: BackendNone,
		Repo: "docling-project/SmolDocling-256M-preview", Revision: "ce51f56c4ebe36e0b1c3a55f67b261ba22a50bf8",
		Publisher: "docling-project (official)", License: "CDLA-Permissive-2.0",
		Description:  "Converts page images into structured DocTags (layout, tables, code, formulas).",
		Capabilities: []string{"Document conversion"},
		Unsupported:  "Published only as Transformers weights; there is no official GGUF and SmolChat has no document-image workflow yet.",
	},
	{
		ID: "colsmol-256m", Name: "ColSmol 256M", Family: "ColSmol", Params: "256M", ParamsM: 256,
		Category: CategoryEmbedding, Backend: BackendNone,
		Repo: "vidore/colSmol-256M", Revision: "c79b633e17e060cc109b11aa2bf92a1517d3bd6f",
		Publisher: "vidore (official)", License: "MIT",
		Description:  "ColPali-style multi-vector embeddings for visual document retrieval.",
		Capabilities: []string{"Document retrieval"},
		Unsupported:  "A ColPali/PEFT model with no llama.cpp runtime; on-device retrieval is not available yet.",
	},
	{
		ID: "colsmol-500m", Name: "ColSmol 500M", Family: "ColSmol", Params: "500M", ParamsM: 500,
		Category: CategoryEmbedding, Backend: BackendNone,
		Repo: "vidore/colSmol-500M", Revision: "0aaa9726104ce485884c7b8faa8a58a72d5fdbe7",
		Publisher: "vidore (official)", License: "MIT",
		Description:  "Larger ColSmol retriever for visual documents.",
		Capabilities: []string{"Document retrieval"},
		Unsupported:  "A ColPali/PEFT model with no llama.cpp runtime; on-device retrieval is not available yet.",
	},
}

func vlm(id, name, family, params string, pm int, repo, rev, quant string, files ...ModelFile) ModelSpec {
	return ModelSpec{
		ID: id, Name: name, Family: family, Params: params, ParamsM: pm,
		Category: CategoryVision, Backend: BackendLlamaMTMD,
		Repo: repo, Revision: rev, Publisher: "ggml-org (llama.cpp maintainers)", License: "Apache-2.0",
		Quant:        quant,
		Description:  "Answers questions about images. Needs both the language model and its vision projector (mmproj).",
		Capabilities: []string{"Image Q&A", "Captioning"},
		Files:        files,
		Advanced:     pm > 1000,
		Unsupported:  "Image input is not available in this version of SmolChat yet.",
	}
}

// FindSpec returns the catalog entry for id.
func FindSpec(id string) (ModelSpec, bool) {
	for _, m := range Catalog {
		if m.ID == id {
			return m, true
		}
	}
	return ModelSpec{}, false
}

// InstalledModel is the manifest written after a verified download.
type InstalledModel struct {
	ID          string      `json:"id"`
	Repo        string      `json:"repo"`
	Revision    string      `json:"revision"`
	Files       []ModelFile `json:"files"`
	InstalledAt time.Time   `json:"installed_at"`
	Verified    bool        `json:"verified"`
}

// ModelManager tracks which catalog models are installed on disk.
type ModelManager struct {
	paths Paths

	mu        sync.Mutex
	installed map[string]InstalledModel
}

// NewModelManager scans the model directory.
func NewModelManager(p Paths) *ModelManager {
	m := &ModelManager{paths: p, installed: map[string]InstalledModel{}}
	m.Scan()
	return m
}

func (m *ModelManager) manifestPath(id string) string {
	return filepath.Join(m.paths.Meta, "installed-"+id+".json")
}

// Dir is where id's files live.
func (m *ModelManager) Dir(id string) string { return filepath.Join(m.paths.Models, id) }

// Scan rediscovers installed models: a model counts as installed only when
// its manifest exists and every file is present with the expected size.
func (m *ModelManager) Scan() {
	found := map[string]InstalledModel{}
	for _, spec := range Catalog {
		b, err := os.ReadFile(m.manifestPath(spec.ID))
		if err != nil {
			continue
		}
		var im InstalledModel
		if json.Unmarshal(b, &im) != nil {
			continue
		}
		ok := len(im.Files) > 0
		for _, f := range im.Files {
			st, err := os.Stat(filepath.Join(m.Dir(spec.ID), f.Name))
			if err != nil || st.Size() != f.Size {
				ok = false
				break
			}
		}
		if ok {
			found[spec.ID] = im
		}
	}
	m.mu.Lock()
	m.installed = found
	m.mu.Unlock()
}

// IsInstalled reports whether id is ready to load.
func (m *ModelManager) IsInstalled(id string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.installed[id]
	return ok
}

// Installed returns installed, supported catalog specs ordered by size.
func (m *ModelManager) Installed() []ModelSpec {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []ModelSpec
	for _, spec := range Catalog {
		if _, ok := m.installed[spec.ID]; ok && spec.Supported {
			out = append(out, spec)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].ParamsM < out[j].ParamsM })
	return out
}

// InstalledBytes is the disk space used by installed models.
func (m *ModelManager) InstalledBytes() int64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	var n int64
	for _, im := range m.installed {
		for _, f := range im.Files {
			n += f.Size
		}
	}
	return n
}

// WeightsPath is the GGUF to load for spec.
func (m *ModelManager) WeightsPath(spec ModelSpec) string {
	return filepath.Join(m.Dir(spec.ID), spec.Weights().Name)
}

// register records a verified install (called by the downloader).
func (m *ModelManager) register(spec ModelSpec) error {
	im := InstalledModel{ID: spec.ID, Repo: spec.Repo, Revision: spec.Revision, Files: spec.Files, InstalledAt: time.Now(), Verified: true}
	if err := writeJSONAtomic(m.manifestPath(spec.ID), im); err != nil {
		return err
	}
	m.mu.Lock()
	m.installed[spec.ID] = im
	m.mu.Unlock()
	return nil
}

// Delete removes id's files and manifest. The caller must unload it first.
func (m *ModelManager) Delete(id string) error {
	if _, ok := FindSpec(id); !ok {
		return fmt.Errorf("unknown model %q", id)
	}
	m.mu.Lock()
	delete(m.installed, id)
	m.mu.Unlock()
	err1 := os.Remove(m.manifestPath(id))
	if errors.Is(err1, os.ErrNotExist) {
		err1 = nil
	}
	err2 := os.RemoveAll(m.Dir(id))
	return errors.Join(err1, err2)
}
