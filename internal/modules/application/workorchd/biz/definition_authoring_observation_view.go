package biz

import (
	"encoding/base64"

	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/workorchmanagementcontract"
)

func authoringProviderObservationMetadata(observation *workspace.DefinitionProviderObservation) *workorchmanagementcontract.DefinitionProviderObservationMetadata {
	if observation == nil {
		return nil
	}
	return &workorchmanagementcontract.DefinitionProviderObservationMetadata{Observed: observation.Observed, Complete: observation.Complete, Saved: observation.Saved, ObservedBytes: observation.ObservedBytes, SHA256: observation.SHA256, Reason: observation.Reason}
}

func authoringProviderObservationView(observation *workspace.DefinitionProviderObservation) *workorchmanagementcontract.DefinitionProviderObservationView {
	metadata := authoringProviderObservationMetadata(observation)
	if metadata == nil {
		return nil
	}
	view := &workorchmanagementcontract.DefinitionProviderObservationView{DefinitionProviderObservationMetadata: *metadata}
	if observation.Saved {
		view.ContentBase64 = base64.StdEncoding.EncodeToString(observation.Content)
	}
	return view
}
