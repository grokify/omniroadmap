package main

import (
	"encoding/json"
	"fmt"
	"os"

	omniroadmap "github.com/grokify/omniroadmap-core"
	"github.com/grokify/omniroadmap-core/provider"
	"github.com/spf13/cobra"

	"github.com/grokify/omniroadmap/augment"
)

// augmentCmd manages locally-authored data (MoSCoW, Kano, RICE, OKR refs,
// notes) layered on top of synced items. Augments are keyed by
// (provider, source ref) — e.g. an Aha reference like MYPROJ-123 — and are
// never touched by sync, so they survive provider re-syncs.
func augmentCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "augment",
		Short: "Manage local augmentations (MoSCoW, Kano, RICE, OKR refs, notes) that survive re-syncs",
	}
	cmd.AddCommand(augmentSetCmd(), augmentGetCmd(), augmentListCmd(), augmentRmCmd())
	return cmd
}

func augmentSetCmd() *cobra.Command {
	flags := &storeFlags{}
	var (
		providerName string
		sourceRef    string
		moscow       string
		kano         string
		reach        float64
		impact       float64
		confidence   float64
		effort       float64
		score        float64
		okrRefs      []string
		notes        string
	)

	cmd := &cobra.Command{
		Use:   "set",
		Short: "Set augment fields for one item (merges with any existing augment)",
		Long: `Set augment fields for one item, keyed by provider and source ref
(e.g. an Aha reference number like MYPROJ-123).

Only the flags you pass change; other augment fields are preserved.
Pass an empty value (--moscow "") to clear a field. Augments are stored
separately from synced provider data and survive every re-sync.`,
		Example: `  omniroadmap augment set --provider aha-studio --ref MYPROJ-123 --moscow must_have
  omniroadmap augment set --provider aha-studio --ref MYPROJ-123 --kano performance \
    --reach 5000 --impact 2 --confidence 0.8 --effort 3
  omniroadmap augment set --provider aha-studio --ref MYPROJ-123 --okr OKR-2026-Q3-01 --notes "exec ask"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			s, _, err := flags.open(cmd.Context())
			if err != nil {
				return err
			}
			defer func() { _ = s.Close() }()

			ctx := cmd.Context()
			aug := augment.ItemAugment{Provider: providerName, SourceRef: sourceRef}
			if existing, err := s.GetItemAugment(ctx, providerName, sourceRef); err == nil {
				aug = *existing
			} else if !omniroadmap.IsNotFound(err) {
				return err
			}

			changed := cmd.Flags().Changed
			if changed("moscow") {
				aug.MoSCoW = moscow
			}
			if changed("kano") {
				aug.Kano = kano
			}
			if changed("okr") {
				aug.OKRRefs = okrRefs
			}
			if changed("notes") {
				aug.Notes = notes
			}
			setRICE := func(flag string, value float64, pick func(*provider.RICE) **float64) {
				if !changed(flag) {
					return
				}
				if aug.RICE == nil {
					aug.RICE = &provider.RICE{}
				}
				v := value
				*pick(aug.RICE) = &v
			}
			setRICE("reach", reach, func(r *provider.RICE) **float64 { return &r.Reach })
			setRICE("impact", impact, func(r *provider.RICE) **float64 { return &r.Impact })
			setRICE("confidence", confidence, func(r *provider.RICE) **float64 { return &r.Confidence })
			setRICE("effort", effort, func(r *provider.RICE) **float64 { return &r.Effort })
			setRICE("score", score, func(r *provider.RICE) **float64 { return &r.Score })

			if err := s.SetItemAugment(ctx, aug); err != nil {
				return err
			}
			if err := s.Commit(ctx, fmt.Sprintf("omniroadmap augment: %s", aug.Key())); err != nil {
				return err
			}
			return printJSON(cmd, aug)
		},
	}

	cmd.Flags().StringVar(&providerName, "provider", "", "Provider the item was synced from (e.g. aha-studio)")
	cmd.Flags().StringVar(&sourceRef, "ref", "", "Source reference (e.g. MYPROJ-123) or source ID")
	cmd.Flags().StringVar(&moscow, "moscow", "", "MoSCoW: must_have|should_have|could_have|wont_have (\"\" clears)")
	cmd.Flags().StringVar(&kano, "kano", "", "Kano: must-be|performance|attractive|indifferent|reverse|questionable (\"\" clears)")
	cmd.Flags().Float64Var(&reach, "reach", 0, "RICE reach")
	cmd.Flags().Float64Var(&impact, "impact", 0, "RICE impact multiplier")
	cmd.Flags().Float64Var(&confidence, "confidence", 0, "RICE confidence multiplier")
	cmd.Flags().Float64Var(&effort, "effort", 0, "RICE effort")
	cmd.Flags().Float64Var(&score, "score", 0, "RICE score override")
	cmd.Flags().StringArrayVar(&okrRefs, "okr", nil, "OKR reference (repeatable; replaces the stored list)")
	cmd.Flags().StringVar(&notes, "notes", "", "Free-text notes (\"\" clears)")
	flags.register(cmd)
	_ = cmd.MarkFlagRequired("provider")
	_ = cmd.MarkFlagRequired("ref")
	return cmd
}

func augmentGetCmd() *cobra.Command {
	flags := &storeFlags{}
	var providerName, sourceRef string

	cmd := &cobra.Command{
		Use:   "get",
		Short: "Show the stored augment for one item",
		RunE: func(cmd *cobra.Command, args []string) error {
			s, _, err := flags.open(cmd.Context())
			if err != nil {
				return err
			}
			defer func() { _ = s.Close() }()

			aug, err := s.GetItemAugment(cmd.Context(), providerName, sourceRef)
			if err != nil {
				return err
			}
			return printJSON(cmd, aug)
		},
	}
	cmd.Flags().StringVar(&providerName, "provider", "", "Provider the item was synced from")
	cmd.Flags().StringVar(&sourceRef, "ref", "", "Source reference (e.g. MYPROJ-123)")
	flags.register(cmd)
	_ = cmd.MarkFlagRequired("provider")
	_ = cmd.MarkFlagRequired("ref")
	return cmd
}

func augmentListCmd() *cobra.Command {
	flags := &storeFlags{}
	var providerName string

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List stored augments, optionally for one provider",
		RunE: func(cmd *cobra.Command, args []string) error {
			s, _, err := flags.open(cmd.Context())
			if err != nil {
				return err
			}
			defer func() { _ = s.Close() }()

			augs, err := s.ListItemAugments(cmd.Context(), providerName)
			if err != nil {
				return err
			}
			if len(augs) == 0 {
				fmt.Println("No augments stored. Add one with: omniroadmap augment set --provider <name> --ref <REF> ...")
				return nil
			}
			return printJSON(cmd, augs)
		},
	}
	cmd.Flags().StringVar(&providerName, "provider", "", "Filter to one provider (default: all)")
	flags.register(cmd)
	return cmd
}

func augmentRmCmd() *cobra.Command {
	flags := &storeFlags{}
	var providerName, sourceRef string

	cmd := &cobra.Command{
		Use:   "rm",
		Short: "Remove the stored augment for one item",
		RunE: func(cmd *cobra.Command, args []string) error {
			s, _, err := flags.open(cmd.Context())
			if err != nil {
				return err
			}
			defer func() { _ = s.Close() }()

			ctx := cmd.Context()
			if err := s.DeleteItemAugment(ctx, providerName, sourceRef); err != nil {
				return err
			}
			if err := s.Commit(ctx, fmt.Sprintf("omniroadmap augment rm: %s:%s", providerName, sourceRef)); err != nil {
				return err
			}
			fmt.Printf("Removed augment %s:%s\n", providerName, sourceRef)
			return nil
		},
	}
	cmd.Flags().StringVar(&providerName, "provider", "", "Provider the item was synced from")
	cmd.Flags().StringVar(&sourceRef, "ref", "", "Source reference (e.g. MYPROJ-123)")
	flags.register(cmd)
	_ = cmd.MarkFlagRequired("provider")
	_ = cmd.MarkFlagRequired("ref")
	return cmd
}

// itemCmd reads merged items (synced provider data + augment overlay) back
// from the store.
func itemCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "item",
		Short: "Read canonical items from the store (augments applied)",
	}
	cmd.AddCommand(itemGetCmd())
	return cmd
}

func itemGetCmd() *cobra.Command {
	flags := &storeFlags{}
	var providerName string

	cmd := &cobra.Command{
		Use:   "get <ref>",
		Short: "Show one item by source reference (e.g. MYPROJ-123), source ID, or canonical ID",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, _, err := flags.open(cmd.Context())
			if err != nil {
				return err
			}
			defer func() { _ = s.Close() }()

			it, err := s.GetItem(cmd.Context(), providerName, args[0])
			if err != nil {
				return err
			}
			return printJSON(cmd, it)
		},
	}
	cmd.Flags().StringVar(&providerName, "provider", "", "Provider to search (default: all; required only when a ref is ambiguous)")
	flags.register(cmd)
	return cmd
}

// printJSON writes v as indented JSON to stdout.
func printJSON(_ *cobra.Command, v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
