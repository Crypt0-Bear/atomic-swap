// SPDX-License-Identifier: LGPLv3

pragma solidity ^0.8.5;

import "./Secp256k1.sol";

contract Swap {
    // Ed25519 library
    Secp256k1 immutable secp256k1;

    // contract creator, Alice
    address payable immutable owner;

    // address allowed to claim the ether in this contract
    address payable immutable claimer;

    // the keccak256 hash of the expected public key derived from the secret `s_b`.
    // this public key is a point on the secp256k1 curve
    bytes32 public immutable pubKeyClaim;

    // the keccak256 hash of the expected public key derived from the secret `s_a`.
    // this public key is a point on the secp256k1 curve
    bytes32 public immutable pubKeyRefund;

    // timestamp (set at contract creation)
    // before which Alice can call either set_ready or refund
    uint256 public immutable timeout_0;

    // timestamp after which Bob cannot claim, only Alice can refund.
    uint256 public immutable timeout_1;

    // Alice sets ready to true when she sees the funds locked on the other chain.
    // this prevents Bob from withdrawing funds without locking funds on the other chain first
    bool public isReady = false;

    event Constructed(bytes32 claimKey, bytes32 refundKey);
    event Ready(bool b);
    event Claimed(bytes32 s);
    event Refunded(bytes32 s);

    constructor(bytes32 _pubKeyClaim, bytes32 _pubKeyRefund, address payable _claimer, uint256 _timeoutDuration) payable {
        owner = payable(msg.sender);
        pubKeyClaim = _pubKeyClaim;
        pubKeyRefund = _pubKeyRefund;
        claimer = _claimer;
        timeout_0 = block.timestamp + _timeoutDuration;
        timeout_1 = block.timestamp + (_timeoutDuration * 2);
        secp256k1 = new Secp256k1();
        emit Constructed(_pubKeyClaim, _pubKeyRefund);
    }

    // Alice must call set_ready() within t_0 once she verifies the XMR has been locked
    function set_ready() external {
        require(!isReady && msg.sender == owner);
        isReady = true;
        emit Ready(true);
    }

    // Bob can claim if:
    // - Alice doesn't call set_ready or refund within t_0, or
    // - Alice calls ready within t_0, in which case Bob can call claim until t_1
    function claim(bytes32 _s) external {
        require(msg.sender == claimer, "only claimer can claim!");
        require((block.timestamp >= timeout_0 || isReady), "too early to claim!");
        require(block.timestamp < timeout_1, "too late to claim!");

        verifySecret(_s, pubKeyClaim);
        emit Claimed(_s);

        // send eth to caller (Bob)
        //selfdestruct(payable(msg.sender));
        claimer.transfer(address(this).balance);
    }

    // Alice can claim a refund:
    // - Until t_0 unless she calls set_ready
    // - After t_1, if she called set_ready
    function refund(bytes32 _s) external {
        require(msg.sender == owner);
        require(
            block.timestamp >= timeout_1 || ( block.timestamp < timeout_0 && !isReady),
            "It's Bob's turn now, please wait!"
        );

        verifySecret(_s, pubKeyRefund);
        emit Refunded(_s);

        // send eth back to owner==caller (Alice)
        //selfdestruct(owner);
        owner.transfer(address(this).balance);
    }

    function verifySecret(bytes32 _s, bytes32 pubKey) internal view {
        require(
            secp256k1.mulVerify(uint256(_s), uint256(pubKey)),
            "provided secret does not match the expected pubKey"
        );
    }
}
