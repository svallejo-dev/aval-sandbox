# orders Specification

## Purpose
Governs the order aggregate of the orders service: how an order is created from its lines, how its total is computed, which status changes it accepts and how the HTTP API exposes it. Money is in minor currency units.

## Requirements

### Requirement: ORD-F01 Order total equals the sum of its lines
The system SHALL compute the total of an order as the sum, over its lines, of quantity times unit price, both when the order is created and after a line is added.

#### Scenario: Several lines
- **WHEN** an order has a line of 2 units at 1500 and a line of 1 unit at 250
- **THEN** its total is 3250

#### Scenario: Line added to a pending order
- **WHEN** a line of 3 units at 100 is added to a pending order whose total is 3250
- **THEN** its total becomes 3550

### Requirement: ORD-F02 Order changes status along allowed edges
The system SHALL move an order only from pending to paid, from pending to cancelled, from paid to shipped and from paid to cancelled. Any other status change MUST be rejected with 409 Conflict and leave the status unchanged.

#### Scenario: Pending order is paid
- **WHEN** a client changes a pending order to paid
- **THEN** the system answers 200 OK and the order status is paid

#### Scenario: Pending order skips to shipped
- **WHEN** a client changes a pending order to shipped
- **THEN** the system answers 409 Conflict and the order stays pending

### Requirement: ORD-F03 Order is readable by its ID
**aval**: characterization
The system SHALL return an order with its status, lines and total when a client reads it by its ID, and SHALL answer 404 Not Found for an ID that no order has.

#### Scenario: Known order
- **WHEN** a client reads an order it created
- **THEN** the system answers 200 OK with the same ID, status, lines and total

#### Scenario: Unknown order
- **WHEN** a client reads an ID that no order has
- **THEN** the system answers 404 Not Found

### Requirement: ORD-F04 Creating an order returns its total
The system SHALL create a pending order from at least one valid line, answer 201 Created with a Location header that points to the new order, and return the order with its total.

#### Scenario: Valid order
- **WHEN** a client creates an order with one line of 2 units at 1500
- **THEN** the system answers 201 Created, the Location header points to the new order and the body shows status pending and total 3000

### Requirement: ORD-N01 Order total is never negative
The system MUST NOT accept a line whose quantity is below 1 or whose unit price is negative, whether the line comes with a new order or is added later, so that no order total is ever negative.

#### Scenario: Negative unit price
- **WHEN** a client creates an order with a line whose unit price is -100
- **THEN** the system answers 422 Unprocessable Content and stores no order

#### Scenario: Zero quantity
- **WHEN** a line with quantity 0 is added to a pending order
- **THEN** the system rejects the line and the order total does not change

### Requirement: ORD-N02 Closed orders never change status
The system MUST NOT change the status of a shipped or a cancelled order.

#### Scenario: Shipped order is cancelled
- **WHEN** a client changes a shipped order to cancelled
- **THEN** the system answers 409 Conflict and the order stays shipped

### Requirement: ORD-N03 Lines are added only while pending
The system MUST NOT add a line to an order that is not pending.

#### Scenario: Paid order gets a line
- **WHEN** a client adds a line to a paid order
- **THEN** the system answers 409 Conflict and the order lines and total do not change

### Requirement: ORD-I01 Order invariants hold for any sequence
For every sequence of line additions and status changes, valid or not, the system SHALL keep the total of an order equal to the sum of its lines and never negative, and SHALL only move its status along the allowed edges.

#### Scenario: Random sequence of operations
- **WHEN** any sequence of line additions and status changes is applied to an order
- **THEN** after every step the total equals the sum of the lines, is not negative, and the status was reached from pending through allowed edges only
