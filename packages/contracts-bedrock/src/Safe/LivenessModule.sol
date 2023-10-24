// SPDX-License-Identifier: MIT
pragma solidity 0.8.15;

import { Safe, OwnerManager } from "safe-contracts/Safe.sol";
import { Enum } from "safe-contracts/common/Enum.sol";
import { OwnerManager } from "safe-contracts/base/OwnerManager.sol";
import { LivenessGuard } from "src/Safe/LivenessGuard.sol";
import { ISemver } from "src/universal/ISemver.sol";

// TODO(maurelian): remove me
import { console2 as console } from "forge-std/console2.sol";

/// @title LivenessModule
/// @notice This module is intended to be used in conjunction with the LivenessGuard. In the event
///         that an owner of the safe is not recorded by the guard during the liveness interval,
///         the owner will be considered inactive and will be removed from the list of owners.
///         If the number of owners falls below the minimum number of owners, the ownership of the
///         safe will be transferred to the fallback owner.
contract LivenessModule is ISemver {
    /// @notice The Safe contract instance
    Safe internal immutable SAFE;

    /// @notice The LivenessGuard contract instance
    ///         This can be updated by replacing with a new module and switching out the guard.
    LivenessGuard internal immutable LIVENESS_GUARD;

    /// @notice The interval, in seconds, during which an owner must have demonstrated liveness
    ///         This can be updated by replacing with a new module.
    uint256 internal immutable LIVENESS_INTERVAL;

    /// @notice The minimum number of owners before ownership of the safe is transferred to the fallback owner.
    ///         This can be updated by replacing with a new module.
    uint256 internal immutable MIN_OWNERS;

    /// @notice The fallback owner of the Safe
    ///         This can be updated by replacing with a new module.
    address internal immutable FALLBACK_OWNER;

    /// @notice The storage slot used in the safe to store the guard address
    ///         keccak256("guard_manager.guard.address")
    uint256 internal constant GUARD_STORAGE_SLOT = 0x4a204f620c8c5ccdca3fd54d003badd85ba500436a431f0cbda4f558c93c34c8;

    /// @notice The address of the first owner in the linked list of owners
    address internal constant SENTINEL_OWNERS = address(0x1);

    /// @notice Semantic version.
    /// @custom:semver 1.0.0
    string public constant version = "1.0.0";

    // Constructor to initialize the Safe and baseModule instances
    constructor(
        Safe _safe,
        LivenessGuard _livenessGuard,
        uint256 _livenessInterval,
        uint256 _minOwners,
        address _fallbackOwner
    ) {
        SAFE = _safe;
        LIVENESS_GUARD = _livenessGuard;
        LIVENESS_INTERVAL = _livenessInterval;
        MIN_OWNERS = _minOwners;
        FALLBACK_OWNER = _fallbackOwner;
    }

    /// @notice This function can be called by anyone to remove an owner that has not signed a transaction
    ///         during the liveness interval. If the number of owners drops below the minimum, then the
    ///         ownership of the Safe is transferred to the fallback owner.
    function removeOwner(address owner) external {
        // Check that the owner to remove has not signed a transaction in the last 30 days
        require(
            LIVENESS_GUARD.lastLive(owner) < block.timestamp - LIVENESS_INTERVAL,
            "LivenessModule: owner has signed recently"
        );

        // Calculate the new threshold
        address[] memory owners = SAFE.getOwners();
        uint256 numOwnersAfter = owners.length - 1;
        if (_isAboveMinOwners(numOwnersAfter)) {
            // Call the Safe to remove the owner and update the threshold
            uint256 thresholdAfter = get75PercentThreshold(numOwnersAfter);
            address prevOwner = _getPrevOwner(owner, owners);
            _removeOwner({ _prevOwner: prevOwner, _owner: owner, _threshold: thresholdAfter });
        } else {
            // The number of owners is dangerously low, so we wish to transfer the ownership of this Safe
            // to the fallback owner.

            // Remove owners one at a time starting from the last owner.
            // Since we're removing them in order from last to first, the ordering will remain constant,
            // and we shouldn't need to query the list of owners again.
            address prevOwner;
            for (uint256 i = owners.length - 1; i > 0; i--) {
                address currentOwner = owners[i];
                prevOwner = _getPrevOwner(currentOwner, owners);

                // Call the Safe to remove the owner
                _removeOwner({ _prevOwner: prevOwner, _owner: currentOwner, _threshold: 1 });
            }

            prevOwner = _getPrevOwner(owners[0], owners);
            // Add the fallback owner as the sole owner of the Safe
            _swapToFallbackOwner({ _prevOwner: prevOwner, _oldOwner: owners[0] });
        }

        _verifyFinalState();
    }

    /// @notice Sets the fallback owner as the sole owner of the Safe with a threshold of 1
    /// @param _prevOwner Owner that pointed to the owner to be replaced in the linked list
    /// @param _oldOwner Owner address to be replaced.
    function _swapToFallbackOwner(address _prevOwner, address _oldOwner) internal {
        SAFE.execTransactionFromModule({
            to: address(SAFE),
            value: 0,
            operation: Enum.Operation.Call,
            data: abi.encodeCall(OwnerManager.swapOwner, (_prevOwner, _oldOwner, FALLBACK_OWNER))
        });
    }

    /// @notice Removes the owner `owner` from the Safe and updates the threshold to `_threshold`.
    /// @param _prevOwner Owner that pointed to the owner to be removed in the linked list
    /// @param _owner Owner address to be removed.
    /// @param _threshold New threshold.
    function _removeOwner(address _prevOwner, address _owner, uint256 _threshold) internal {
        SAFE.execTransactionFromModule({
            to: address(SAFE),
            value: 0,
            operation: Enum.Operation.Call,
            data: abi.encodeCall(OwnerManager.removeOwner, (_prevOwner, _owner, _threshold))
        });
    }

    /// @notice A FREI-PI invariant check enforcing requirements on number of owners and threshold.
    function _verifyFinalState() internal view {
        address[] memory owners = SAFE.getOwners();
        uint256 numOwners = owners.length;
        require(
            _isAboveMinOwners(numOwners) || (numOwners == 1 && owners[0] == FALLBACK_OWNER),
            "LivenessModule: Safe must have the minimum number of owners or be owned solely by the fallback owner"
        );

        // Check that the threshold is correct. This check is also correct when there is a single
        // owner, because get75PercentThreshold(1) returns 1.
        uint256 threshold = SAFE.getThreshold();
        require(
            threshold == get75PercentThreshold(numOwners),
            "LivenessModule: threshold must be 75% of the number of owners"
        );

        // Check that the guard has not been changed.
        _verifyGuard();
    }

    /// @notice Reverts if the guard address does not match the expected value.
    function _verifyGuard() internal view {
        require(
            address(LIVENESS_GUARD) == address(uint160(uint256(bytes32(SAFE.getStorageAt(GUARD_STORAGE_SLOT, 1))))),
            "LivenessModule: guard has been changed"
        );
    }

    /// @notice Get the previous owner in the linked list of owners
    /// @param _owner The owner whose previous owner we want to find
    /// @param _owners The list of owners
    function _getPrevOwner(address _owner, address[] memory _owners) internal pure returns (address prevOwner_) {
        for (uint256 i = 0; i < _owners.length; i++) {
            if (_owners[i] != _owner) continue;
            if (i == 0) {
                prevOwner_ = SENTINEL_OWNERS;
                break;
            }
            prevOwner_ = _owners[i - 1];
        }
    }

    /// @notice For a given number of owners, return the lowest threshold which is greater than 75.
    ///         Note: this function returns 1 for numOwners == 1.
    function get75PercentThreshold(uint256 _numOwners) public pure returns (uint256 threshold_) {
        threshold_ = (_numOwners * 75 + 99) / 100;
    }

    /// @notice Check if the number of owners is greater than or equal to the minimum number of owners.
    /// @param numOwners The number of owners.
    /// @return A boolean indicating if the number of owners is greater than or equal to the minimum number of owners.
    function _isAboveMinOwners(uint256 numOwners) internal view returns (bool) {
        return numOwners >= MIN_OWNERS;
    }

    /// @notice Getter function for the Safe contract instance
    /// @return safe_ The Safe contract instance
    function safe() public view returns (Safe safe_) {
        safe_ = SAFE;
    }

    /// @notice Getter function for the LivenessGuard contract instance
    /// @return livenessGuard_ The LivenessGuard contract instance
    function livenessGuard() public view returns (LivenessGuard livenessGuard_) {
        livenessGuard_ = LIVENESS_GUARD;
    }

    /// @notice Getter function for the liveness interval
    /// @return livenessInterval_ The liveness interval, in seconds
    function livenessInterval() public view returns (uint256 livenessInterval_) {
        livenessInterval_ = LIVENESS_INTERVAL;
    }

    /// @notice Getter function for the minimum number of owners
    /// @return minOwners_ The minimum number of owners
    function minOwners() public view returns (uint256 minOwners_) {
        minOwners_ = MIN_OWNERS;
    }

    /// @notice Getter function for the fallback owner
    /// @return fallbackOwner_ The fallback owner of the Safe
    function fallbackOwner() public view returns (address fallbackOwner_) {
        fallbackOwner_ = FALLBACK_OWNER;
    }
}