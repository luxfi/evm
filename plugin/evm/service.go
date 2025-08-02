// (c) 2020-2020, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package evm

import (
	"net/http"

	"github.com/luxfi/evm/v2/v2/plugin/evm/client"
)

type ValidatorsAPI struct {
	vm *VM
}

func (api *ValidatorsAPI) GetCurrentValidators(_ *http.Request, req *client.GetCurrentValidatorsRequest, reply *client.GetCurrentValidatorsResponse) error {
	api.vm.vmLock.RLock()
	defer api.vm.vmLock.RUnlock()

	// TODO: Implement validator tracking
	// For now, return empty validators list
	reply.Validators = []client.CurrentValidator{}
	return nil

	// Original implementation commented out as validators.Manager doesn't have these methods:
	/*
		var vIDs utils.Set[ids.ID]
		if len(req.NodeIDs) > 0 {
			vIDs = utils.NewSet[ids.ID](len(req.NodeIDs))
			for _, nodeID := range req.NodeIDs {
				vID, err := api.vm.validatorsManager.GetValidationID(nodeID)
				if err != nil {
					return fmt.Errorf("couldn't find validator with node ID %s", nodeID)
				}
				vIDs.Add(vID)
			}
		} else {
			vIDs = api.vm.validatorsManager.GetValidationIDs()
		}

		reply.Validators = make([]client.CurrentValidator, 0, vIDs.Len())

		for _, vID := range vIDs.List() {
			validator, err := api.vm.validatorsManager.GetValidator(vID)
			if err != nil {
				return fmt.Errorf("couldn't find validator with validation ID %s", vID)
			}

			isConnected := api.vm.validatorsManager.IsConnected(validator.NodeID)

			upDuration, lastUpdated, err := api.vm.validatorsManager.CalculateUptime(validator.NodeID)
			if err != nil {
				return err
			}
			var uptimeFloat float64
			startTime := time.Unix(int64(validator.StartTimestamp), 0)
			bestPossibleUpDuration := lastUpdated.Sub(startTime)
			if bestPossibleUpDuration == 0 {
				uptimeFloat = 1
			} else {
				uptimeFloat = float64(upDuration) / float64(bestPossibleUpDuration)
			}

			// Transform this to a percentage (0-100) to make it consistent
			// with currentValidators in PlatformVM API
			uptimePercentage := float32(uptimeFloat * 100)

			reply.Validators = append(reply.Validators, client.CurrentValidator{
				ValidationID:     validator.ValidationID,
				NodeID:           validator.NodeID,
				StartTimestamp:   validator.StartTimestamp,
				Weight:           validator.Weight,
				IsActive:         validator.IsActive,
				IsL1Validator:    validator.IsL1Validator,
				IsConnected:      isConnected,
				UptimePercentage: uptimePercentage,
				UptimeSeconds:    uint64(upDuration.Seconds()),
			})
		}
		return nil
	*/
}
