## ADDED Requirements

### Requirement: Responsive archive exploration interface
The frontend SHALL provide a polished Vue interface with a consistent green-accented visual system, light/dark themes, readable result cards or rows, platform badges, search/filter controls, analytics panels, and a detail drawer. It SHALL adapt from desktop to mobile without losing core actions.

#### Scenario: Browse on desktop
- **WHEN** a user opens a loaded archive
- **THEN** results, active filters, analytics, source, and snapshot freshness are visible in a coherent layout

#### Scenario: Browse on mobile
- **WHEN** the viewport narrows to phone width
- **THEN** panels stack and search, filtering, pagination, and result details remain usable without horizontal page overflow

### Requirement: Linked filters and shareable navigation
The UI SHALL synchronize search, filters, and page state with the browser URL. Typing SHALL be debounced and obsolete responses SHALL NOT replace newer results. Clicking a stored keyword, author, or cloud term SHALL update the shared filter state, reset pagination, and refresh results and analytics together.

#### Scenario: Drill into a keyword
- **WHEN** a user selects a keyword in the visualization
- **THEN** its filter appears as a removable chip and all panels show the same filtered selection

#### Scenario: Restore a shared URL
- **WHEN** a user opens a valid filtered URL or navigates backward/forward
- **THEN** controls, selected page, results, and analytics reflect the encoded state

#### Scenario: Out-of-order search responses
- **WHEN** an older search response arrives after a newer response
- **THEN** it does not overwrite the latest query's results or charts

### Requirement: Honest analytics and data coverage
The UI SHALL show keyword counts, daily recorded-time activity, a word cloud with a count-list alternative, and most-active authors within the collected archive. It SHALL label recorded timestamps, content coverage, and missing author data and SHALL NOT present historical missing metadata as zero real-world activity.

#### Scenario: Legacy archive has titles but no content or authors
- **WHEN** the user opens its analytics
- **THEN** title-based terms remain visible, content/author limitations are explained, and the author panel reports unavailable data rather than invented rankings

#### Scenario: Selected result has rich metadata
- **WHEN** a user opens its detail drawer
- **THEN** available title, plain text, author, stored keyword, platform, timestamp, and source link are shown

### Requirement: Loading, error, empty, and stale states
The UI SHALL distinguish initial loading, empty archive, no filter matches, load-limit failures, database failures, and stale snapshots. It SHALL provide retry/refresh and clear-filters actions where appropriate, preserve successful data during refresh failures, and reset pagination on `snapshot_changed`.

#### Scenario: Database becomes unavailable during refresh
- **WHEN** the backend reports a failed refresh with a prior complete snapshot
- **THEN** prior results stay visible with a stale/error indicator and original refresh time

#### Scenario: No records match
- **WHEN** filters eliminate all records
- **THEN** an explanatory empty state offers clearing filters without implying the archive is empty

### Requirement: Accessible and safe presentation
All core controls and visualization drill-downs SHALL be keyboard accessible with visible focus and textual labels/counts. Content SHALL be rendered as text; source links SHALL only allow HTTP(S). Light/dark presentation SHALL maintain readable contrast and avoid color-only distinctions.

#### Scenario: Keyboard-only exploration
- **WHEN** a user tabs through search, filters, analytics, pagination, and details
- **THEN** each interaction can be completed with visible focus and understandable labels

#### Scenario: Stored content contains executable markup
- **WHEN** a result contains script tags or an unsafe source URL
- **THEN** the UI does not execute markup or navigate to an unsafe scheme
