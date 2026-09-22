# payments Specification

## Purpose
Governs the payment aggregate of the orders service: capturing the amount paid for an order and issuing refunds against it, idempotently per key and never above what was captured. Money is in minor currency units.

## Requirements

### Requirement: ORD-F10 Refund is idempotent per key
The system SHALL issue at most one refund per idempotency key and payment. A refund repeated with the same key and amount MUST return the original refund with 200 OK and refund nothing again.

#### Scenario: Same key and amount
- **WHEN** a client repeats a refund of 300 with the key k1 that already refunded 300
- **THEN** the system answers 200 OK with the original refund and the refunded total stays 300

#### Scenario: Different keys
- **WHEN** a client refunds 300 with the key k1 and then 200 with the key k2
- **THEN** the system answers 201 Created to both and the refunded total is 500

### Requirement: ORD-F11 Payment captures a positive amount
The system SHALL capture a payment for an order when its amount is positive, answering 201 Created, and MUST reject an amount of zero or less with 422 Unprocessable Content.

#### Scenario: Positive amount
- **WHEN** a client captures a payment of 1000 for an order
- **THEN** the system answers 201 Created with captured 1000 and refunded 0

#### Scenario: Zero amount
- **WHEN** a client captures a payment of 0
- **THEN** the system answers 422 Unprocessable Content and stores no payment

### Requirement: ORD-F12 Refund requires an idempotency key
The system SHALL reject a refund request without an Idempotency-Key header, or with a key longer than 128 bytes, with 400 Bad Request, and MUST NOT issue a refund for it.

#### Scenario: Missing key
- **WHEN** a client requests a refund without the Idempotency-Key header
- **THEN** the system answers 400 Bad Request and the refunded total does not change

### Requirement: ORD-N10 Never refund more than captured
The system MUST NOT issue a refund that would take the refunded total of a payment above its captured amount, and SHALL answer 409 Conflict instead.

#### Scenario: Refund above what is left
- **WHEN** a payment captured 1000, 600 is already refunded and a client refunds 500 with a new key
- **THEN** the system answers 409 Conflict and the refunded total stays 600

### Requirement: ORD-N11 Refund key never changes its amount
The system MUST NOT accept an idempotency key again for a refund of a different amount, and SHALL answer 422 Unprocessable Content and leave the original refund as it was.

#### Scenario: Same key, other amount
- **WHEN** a client refunds 300 with the key k1 and then 400 with the key k1
- **THEN** the system answers 422 Unprocessable Content and the refunded total stays 300

### Requirement: ORD-I10 Refunded total never exceeds captured
For every sequence of refund requests, with new and repeated keys, the system SHALL keep the refunded total of a payment equal to the sum of its refunds and never above the captured amount.

#### Scenario: Random sequence of refunds
- **WHEN** any sequence of refunds with new and repeated keys is applied to a payment
- **THEN** after every step the refunded total equals the sum of the refunds and does not exceed the captured amount
