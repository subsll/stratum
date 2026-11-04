// Package casestate replays a workspace's evidence and custody ledgers into the
// state used by commands and reports. The ledgers are authoritative; case.json
// is only a human-friendly marker and is checked for drift.
package casestate

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/subsll/stratum/internal/chain"
	"github.com/subsll/stratum/internal/custody"
	"github.com/subsll/stratum/internal/model"
	"github.com/subsll/stratum/internal/workspace"
)

// State is the interpreted state of one case.
type State struct {
	WS            *workspace.Workspace
	Case          model.Case
	CaseRecord    chain.Record
	Items         []model.Item
	ItemIndex     map[string]int
	Issues        []model.Issue
	Events        []chain.Record
	Evidence      *chain.LoadResult
	Custody       *chain.LoadResult
	Ledger        *custody.Ledger
	Sealed        bool
	SealedAt      string
	Verifications []model.VerificationData
	Warnings      []string
}

// Load replays both ledgers and returns the case state. Broken chains do not
// prevent loading parseable records, but callers must check EvidenceOK and
// CustodyOK before appending.
func Load(ws *workspace.Workspace) (*State, error) {
	ev, err := chain.Load(ws.EvidencePath)
	if err != nil {
		return nil, fmt.Errorf("load evidence ledger: %w", err)
	}
	st := &State{
		WS:        ws,
		Evidence:  ev,
		ItemIndex: make(map[string]int),
	}
	if len(ev.Records) == 0 {
		return nil, fmt.Errorf("%s contains no evidence records", ws.EvidencePath)
	}

	for _, rec := range ev.Records {
		st.Events = append(st.Events, rec)
		switch rec.Type {
		case "case_created":
			var data model.CaseCreatedData
			if err := json.Unmarshal(rec.Data, &data); err != nil {
				st.Warnings = append(st.Warnings, fmt.Sprintf("case_created record %d is unreadable: %v", rec.Seq, err))
				continue
			}
			if st.Case.CaseID != "" {
				st.Warnings = append(st.Warnings, fmt.Sprintf("case_created record %d is a second case definition", rec.Seq))
				continue
			}
			st.Case = model.Case{
				FormatVersion: model.FormatVersion,
				CaseID:        data.CaseID,
				Name:          data.Name,
				Slug:          data.Slug,
				Description:   data.Description,
				Examiner:      data.Examiner,
				Created:       rec.Time,
				Tool:          model.ToolName,
				Version:       data.Environment.Version,
			}
			st.CaseRecord = rec
			if rec.Target != data.CaseID {
				st.Warnings = append(st.Warnings, fmt.Sprintf("case_created record %d target %q does not match case id %q", rec.Seq, rec.Target, data.CaseID))
			}
		case "item_acquired":
			var item model.Item
			if err := json.Unmarshal(rec.Data, &item); err != nil {
				st.Warnings = append(st.Warnings, fmt.Sprintf("item record %d is unreadable: %v", rec.Seq, err))
				continue
			}
			if item.ID == "" {
				st.Warnings = append(st.Warnings, fmt.Sprintf("item record %d has no item id", rec.Seq))
				continue
			}
			if rec.Target != item.ID {
				st.Warnings = append(st.Warnings, fmt.Sprintf("item record %d target %q does not match item id %q", rec.Seq, rec.Target, item.ID))
			}
			if _, exists := st.ItemIndex[item.ID]; exists {
				st.Warnings = append(st.Warnings, fmt.Sprintf("duplicate item id %s at record %d; the first record is used for lookup", item.ID, rec.Seq))
			} else {
				st.ItemIndex[item.ID] = len(st.Items)
			}
			if item.AcquiredAt == "" {
				item.AcquiredAt = rec.Time
			}
			if item.AcquiredBy.Empty() {
				item.AcquiredBy = st.Case.Examiner
			}
			if !validSHA256(item.SHA256) {
				st.Warnings = append(st.Warnings, fmt.Sprintf("item %s has an invalid SHA-256 value", item.ID))
			}
			if item.Metadata.Error != "" {
				st.Warnings = append(st.Warnings, fmt.Sprintf("item %s metadata: %s", item.ID, item.Metadata.Error))
			}
			if item.Document.Error != "" {
				st.Warnings = append(st.Warnings, fmt.Sprintf("item %s document metadata: %s", item.ID, item.Document.Error))
			}
			st.Items = append(st.Items, item)
		case "item_metadata":
			var data model.ItemMetadataData
			if err := json.Unmarshal(rec.Data, &data); err != nil {
				st.Warnings = append(st.Warnings, fmt.Sprintf("item_metadata record %d is unreadable: %v", rec.Seq, err))
				continue
			}
			idx, ok := st.ItemIndex[data.ItemID]
			if !ok {
				st.Warnings = append(st.Warnings, fmt.Sprintf("item_metadata record %d references unknown item %s", rec.Seq, data.ItemID))
				continue
			}
			if rec.Target != data.ItemID {
				st.Warnings = append(st.Warnings, fmt.Sprintf("item_metadata record %d target %q does not match item id %q", rec.Seq, rec.Target, data.ItemID))
			}
			st.Items[idx].Metadata = data.Metadata
			st.Items[idx].EXIF = data.EXIF
			st.Items[idx].Document = data.Document
			if data.Metadata.Error != "" {
				st.Warnings = append(st.Warnings, fmt.Sprintf("item %s metadata: %s", data.ItemID, data.Metadata.Error))
			}
			if data.Document.Error != "" {
				st.Warnings = append(st.Warnings, fmt.Sprintf("item %s document metadata: %s", data.ItemID, data.Document.Error))
			}
		case "item_error":
			var data model.ItemErrorData
			if err := json.Unmarshal(rec.Data, &data); err != nil {
				st.Warnings = append(st.Warnings, fmt.Sprintf("item_error record %d is unreadable: %v", rec.Seq, err))
				continue
			}
			kind := data.Kind
			if kind == "" {
				kind = "error"
			}
			st.Issues = append(st.Issues, model.Issue{Time: rec.Time, Path: data.Path, Kind: kind, Reason: data.Reason, EventHash: rec.Hash})
		case "case_sealed":
			st.Sealed = true
			st.SealedAt = rec.Time
		case "case_unsealed":
			st.Sealed = false
			st.SealedAt = ""
		case "verification":
			var data model.VerificationData
			if err := json.Unmarshal(rec.Data, &data); err == nil {
				st.Verifications = append(st.Verifications, data)
			}
		}
	}
	if st.Case.CaseID == "" {
		return nil, errors.New("evidence ledger has no readable case_created record")
	}

	cu, err := chain.Load(ws.CustodyPath)
	if err != nil {
		return nil, fmt.Errorf("load custody ledger: %w", err)
	}
	st.Custody = cu
	st.Ledger = custody.Analyze(cu.Records, st.Items, st.Case.Examiner.Display())
	st.Warnings = append(st.Warnings, st.Ledger.Warnings...)

	// case.json is a marker, not evidence. Report drift but trust the chain.
	if ws.Case.CaseID != st.Case.CaseID {
		st.Warnings = append(st.Warnings, fmt.Sprintf("case.json case_id %q differs from evidence chain %q", ws.Case.CaseID, st.Case.CaseID))
	}
	if ws.Case.Name != st.Case.Name {
		st.Warnings = append(st.Warnings, "case.json name differs from the evidence chain")
	}
	if ws.Case.Examiner.Name != st.Case.Examiner.Name || ws.Case.Examiner.Org != st.Case.Examiner.Org {
		st.Warnings = append(st.Warnings, "case.json examiner differs from the evidence chain")
	}
	return st, nil
}

// EvidenceOK reports whether the evidence chain verified.
func (s *State) EvidenceOK() bool { return s.Evidence != nil && s.Evidence.Report.OK }

// CustodyOK reports whether the custody chain verified.
func (s *State) CustodyOK() bool { return s.Custody != nil && s.Custody.Report.OK }

// ChainsOK reports whether both chains verified.
func (s *State) ChainsOK() bool { return s.EvidenceOK() && s.CustodyOK() }

// Item returns an item by ID.
func (s *State) Item(id string) (model.Item, bool) {
	i, ok := s.ItemIndex[id]
	if !ok || i < 0 || i >= len(s.Items) {
		return model.Item{}, false
	}
	return s.Items[i], true
}

// ItemIDs returns all item IDs in acquisition order.
func (s *State) ItemIDs() []string {
	out := make([]string, 0, len(s.Items))
	for _, it := range s.Items {
		out = append(out, it.ID)
	}
	return out
}

// TotalBytes returns the sum of acquired item sizes.
func (s *State) TotalBytes() int64 {
	var n int64
	for _, it := range s.Items {
		n += it.Size
	}
	return n
}

// NextItemID returns the next E-nnnn identifier.
func (s *State) NextItemID() string {
	max := 0
	for _, it := range s.Items {
		suffix := strings.TrimPrefix(it.ID, "E-")
		if n, err := strconv.Atoi(suffix); err == nil && n > max {
			max = n
		}
	}
	return fmt.Sprintf("E-%04d", max+1)
}

// Holder returns the current custodian recorded for an item.
func (s *State) Holder(id string) string {
	if s.Ledger == nil {
		return ""
	}
	return s.Ledger.HolderByItem[id]
}

// EvidenceInfo summarises the evidence chain.
func (s *State) EvidenceInfo() model.EvidenceInfo {
	if s.Evidence == nil || s.Evidence.Report == nil {
		return model.EvidenceInfo{}
	}
	r := s.Evidence.Report
	return model.EvidenceInfo{File: workspace.EvidenceFile, Records: r.Records, RootSHA256: r.RootSHA256, FileSHA256: r.FileSHA256, ChainOK: r.OK}
}

// CustodyInfo summarises the custody chain.
func (s *State) CustodyInfo() model.EvidenceInfo {
	if s.Custody == nil || s.Custody.Report == nil {
		return model.EvidenceInfo{}
	}
	r := s.Custody.Report
	return model.EvidenceInfo{File: workspace.CustodyFile, Records: r.Records, RootSHA256: r.RootSHA256, FileSHA256: r.FileSHA256, ChainOK: r.OK}
}

// ChainErrors returns verification errors from both ledgers.
func (s *State) ChainErrors() []string {
	var out []string
	if s.Evidence != nil && s.Evidence.Report != nil {
		for _, e := range s.Evidence.Report.Errors {
			out = append(out, "evidence: "+e)
		}
	}
	if s.Custody != nil && s.Custody.Report != nil {
		for _, e := range s.Custody.Report.Errors {
			out = append(out, "custody: "+e)
		}
	}
	return out
}

func validSHA256(s string) bool {
	if len(s) != 64 {
		return false
	}
	_, err := hex.DecodeString(s)
	return err == nil
}
