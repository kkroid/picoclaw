package prepare

import "strings"

func normalizeInteractionSurfaces(surfaceList []InteractionSurface, screenList []Screen) []InteractionSurface {
	if len(surfaceList) > 0 {
		result := make([]InteractionSurface, 0, len(surfaceList))
		for _, surface := range surfaceList {
			normalized := InteractionSurface{
				SurfaceID:          strings.TrimSpace(surface.SurfaceID),
				Label:              strings.TrimSpace(surface.Label),
				Purpose:            strings.TrimSpace(surface.Purpose),
				PrimaryFeatureRefs: append([]string(nil), surface.PrimaryFeatureRefs...),
				LegacyScreenRef:    strings.TrimSpace(surface.LegacyScreenRef),
			}
			if normalized.SurfaceID == "" {
				normalized.SurfaceID = strings.TrimSpace(normalized.LegacyScreenRef)
			}
			result = append(result, normalized)
		}
		return result
	}
	result := make([]InteractionSurface, 0, len(screenList))
	for _, screen := range screenList {
		result = append(result, InteractionSurface{
			SurfaceID:          strings.TrimSpace(screen.ScreenID),
			Label:              strings.TrimSpace(screen.Name),
			Purpose:            strings.TrimSpace(screen.Purpose),
			PrimaryFeatureRefs: append([]string(nil), screen.PrimaryFeatures...),
			LegacyScreenRef:    strings.TrimSpace(screen.ScreenID),
		})
	}
	return result
}

func publicInteractionSurfaceList(surfaceList []InteractionSurface) []InteractionSurface {
	if len(surfaceList) == 0 {
		return nil
	}
	result := make([]InteractionSurface, 0, len(surfaceList))
	for _, surface := range surfaceList {
		surfaceID := strings.TrimSpace(surface.SurfaceID)
		if surfaceID == "" {
			continue
		}
		result = append(result, InteractionSurface{
			SurfaceID:          surfaceID,
			Label:              strings.TrimSpace(surface.Label),
			Purpose:            strings.TrimSpace(surface.Purpose),
			PrimaryFeatureRefs: uniqueStrings(surface.PrimaryFeatureRefs),
		})
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func deriveCompatibilityScreenList(surfaceList []InteractionSurface, fallback []Screen) []Screen {
	if len(surfaceList) == 0 {
		return append([]Screen(nil), fallback...)
	}
	result := make([]Screen, 0, len(surfaceList))
	for _, surface := range surfaceList {
		screenID := strings.TrimSpace(surface.LegacyScreenRef)
		if screenID == "" {
			screenID = strings.TrimSpace(surface.SurfaceID)
		}
		result = append(result, Screen{
			ScreenID:        screenID,
			Name:            firstNonEmpty(surface.Label, surface.SurfaceID),
			Purpose:         strings.TrimSpace(surface.Purpose),
			PrimaryFeatures: append([]string(nil), surface.PrimaryFeatureRefs...),
		})
	}
	return result
}

func surfaceIDByLegacyScreenRef(surfaceList []InteractionSurface) map[string]string {
	result := make(map[string]string, len(surfaceList))
	for _, surface := range surfaceList {
		legacy := strings.TrimSpace(surface.LegacyScreenRef)
		if legacy == "" {
			continue
		}
		result[legacy] = strings.TrimSpace(surface.SurfaceID)
	}
	return result
}

func surfaceIDSet(surfaceList []InteractionSurface) map[string]struct{} {
	result := make(map[string]struct{}, len(surfaceList))
	for _, surface := range surfaceList {
		surfaceID := strings.TrimSpace(surface.SurfaceID)
		if surfaceID == "" {
			continue
		}
		result[surfaceID] = struct{}{}
	}
	return result
}

func normalizeSurfaceRefsForSchema(surfaceRefs []string, knownSurfaceIDs map[string]struct{}) []string {
	trimmed := uniqueStrings(surfaceRefs)
	if len(trimmed) == 0 {
		return nil
	}
	if len(knownSurfaceIDs) == 0 {
		return trimmed
	}
	result := make([]string, 0, len(trimmed))
	for _, surfaceRef := range trimmed {
		if _, ok := knownSurfaceIDs[surfaceRef]; !ok {
			continue
		}
		result = append(result, surfaceRef)
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func deriveSurfaceRefsFromLegacyScreens(screenRefs []string, surfaceByScreen map[string]string) []string {
	if len(screenRefs) == 0 || len(surfaceByScreen) == 0 {
		return nil
	}
	refs := make([]string, 0, len(screenRefs))
	for _, screenRef := range uniqueStrings(screenRefs) {
		if surfaceRef := strings.TrimSpace(surfaceByScreen[screenRef]); surfaceRef != "" {
			refs = append(refs, surfaceRef)
		}
	}
	return uniqueStrings(refs)
}

func legacyScreenRefBySurfaceID(surfaceList []InteractionSurface) map[string]string {
	result := make(map[string]string, len(surfaceList))
	for _, surface := range surfaceList {
		surfaceID := strings.TrimSpace(surface.SurfaceID)
		if surfaceID == "" {
			continue
		}
		legacy := strings.TrimSpace(surface.LegacyScreenRef)
		if legacy == "" {
			legacy = surfaceID
		}
		result[surfaceID] = legacy
	}
	return result
}

func normalizeFeatureSurfaceRefs(features []Feature, surfaceList []InteractionSurface) []Feature {
	if len(features) == 0 {
		return nil
	}
	surfaceByScreen := surfaceIDByLegacyScreenRef(surfaceList)
	knownSurfaceIDs := surfaceIDSet(surfaceList)
	result := make([]Feature, 0, len(features))
	for _, feature := range features {
		normalized := feature
		normalized.RelatedSurfaceRefs = normalizeSurfaceRefsForSchema(feature.RelatedSurfaceRefs, knownSurfaceIDs)
		if len(normalized.RelatedSurfaceRefs) == 0 && len(normalized.RelatedScreens) > 0 {
			normalized.RelatedSurfaceRefs = deriveSurfaceRefsFromLegacyScreens(feature.RelatedScreens, surfaceByScreen)
		}
		normalized.RelatedScreens = nil
		result = append(result, normalized)
	}
	return result
}

func normalizeUserFlowsForSurfaces(flows []UserFlow, surfaceList []InteractionSurface) []UserFlow {
	if len(flows) == 0 {
		return nil
	}
	surfaceByScreen := surfaceIDByLegacyScreenRef(surfaceList)
	knownSurfaceIDs := surfaceIDSet(surfaceList)
	result := make([]UserFlow, 0, len(flows))
	for _, flow := range flows {
		normalized := flow
		normalized.Steps = make([]FlowStep, 0, len(flow.Steps))
		for _, step := range flow.Steps {
			normalizedStep := step
			normalizedStep.SurfaceRef = strings.TrimSpace(step.SurfaceRef)
			if normalizedStep.SurfaceRef != "" && len(knownSurfaceIDs) > 0 {
				if _, ok := knownSurfaceIDs[normalizedStep.SurfaceRef]; !ok {
					normalizedStep.SurfaceRef = ""
				}
			}
			if normalizedStep.SurfaceRef == "" {
				normalizedStep.SurfaceRef = strings.TrimSpace(surfaceByScreen[strings.TrimSpace(step.ScreenRef)])
			}
			normalizedStep.ScreenRef = ""
			normalized.Steps = append(normalized.Steps, normalizedStep)
		}
		result = append(result, normalized)
	}
	return result
}
