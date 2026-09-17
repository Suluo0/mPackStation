package service

import (
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"

	"mpackstation/internal/store"
)

var declaredModIDPattern = regexp.MustCompile(`^[a-z][a-z0-9_.-]{1,127}$`)

func normalizeDeclaredModID(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if !declaredModIDPattern.MatchString(value) {
		return ""
	}
	return value
}

func measuredArchive(content []byte) (sha1Hex, sha256Hex string) {
	one := sha1.Sum(content)
	two := sha256.Sum256(content)
	return hex.EncodeToString(one[:]), hex.EncodeToString(two[:])
}

func manifestHash(values ...string) string {
	digest := sha256.New()
	for _, value := range values {
		_, _ = digest.Write([]byte(value))
		_, _ = digest.Write([]byte{0})
	}
	return hex.EncodeToString(digest.Sum(nil))
}

func nonEmptyString(value, fallback string) string {
	if value != "" {
		return value
	}
	return fallback
}

func verifiedPackSelection(mod store.PackModRecord, declaredVersion, sha256Hex, location string, size int64, at int64) store.PackScopedSelection {
	versionKey := mod.ModID + "\x00" + declaredVersion + "\x00" + sha256Hex
	versionID := store.NewGlobalID("version", versionKey)
	selectionID := store.NewScopedID("selection", mod.PackID, mod.ID+"\x00"+versionID)
	return store.PackScopedSelection{
		PackID: mod.PackID, PackModID: mod.ID, ModID: mod.ModID,
		SelectionID: selectionID, VersionID: versionID,
		SourceID:        store.NewScopedID("source", mod.PackID, selectionID),
		DeclaredVersion: declaredVersion, ManifestSHA256: sha256Hex,
		Acquisition: mod.Source, RequestedVersion: mod.VersionID,
		Platform: mod.Source, ExternalProjectID: mod.ProjectID,
		ProjectDisplayName: mod.DisplayName, ExternalReleaseID: mod.VersionID,
		ReleaseName: declaredVersion, ReleaseFileKey: mod.FileName, DownloadURL: location,
		File: &store.PackScopedFile{
			ID: store.NewGlobalID("file", sha256Hex), SHA256: sha256Hex, SHA1: mod.SHA1,
			SizeBytes: size, MediaType: "application/java-archive", Location: location, Verified: true,
		},
		LogicalPath: mod.FileName, FileRole: "primary", CreatedAt: at, VersionStatus: "ready",
	}
}

func validateMeasuredDownload(expectedSHA1, expectedSHA256 string, content []byte) (string, string, error) {
	actualSHA1, actualSHA256 := measuredArchive(content)
	if expectedSHA1 != "" && !strings.EqualFold(expectedSHA1, actualSHA1) {
		return "", "", fmt.Errorf("download sha1 mismatch")
	}
	if expectedSHA256 != "" && !strings.EqualFold(expectedSHA256, actualSHA256) {
		return "", "", fmt.Errorf("download sha256 mismatch")
	}
	return actualSHA1, actualSHA256, nil
}
