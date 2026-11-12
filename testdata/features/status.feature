Feature: Status reporting
  Enforcers report usage and receive the current policy decision.

  Background:
    Given a Timetower server with configuration "config/valid.json" at "2024-06-03T10:00:00Z"

  Scenario: A report records usage and returns the decision
    When the enforcer reports:
      | day  | 2024-06-03 |
      | used | 1200       |
    Then the response status is 200
    And the status has a revision
    And the status is:
      | user                   | dani |
      | reported_used_seconds  | 1200 |
      | aggregate_used_seconds | 1200 |
      | remaining_seconds      | 2400 |
      | allowed                | true |
      | deny_reason            |      |
    And the schedule is:
      | used_seconds      | 1200 |
      | remaining_seconds | 2400 |

  Scenario: A lower counter does not replace stored usage
    When the enforcer reports:
      | day  | 2024-06-03 |
      | used | 1200       |
    And the enforcer reports:
      | day  | 2024-06-03 |
      | used | 1000       |
    Then the response status is 200
    And the status is:
      | reported_used_seconds | 1200 |
    And the stored usage for "dani" on "2024-06-03" is 1200 seconds

  Scenario: The daily limit denies further usage
    When the enforcer reports:
      | day  | 2024-06-03 |
      | used | 3600       |
    Then the status is:
      | allowed             | false               |
      | deny_reason         | daily_limit_reached |
      | remaining_seconds   | 0                   |

  Scenario: Reporting another day is rejected
    When the enforcer reports:
      | day  | 2024-06-04 |
      | used | 100        |
    Then the response status is 409

  Scenario: A report without a hostname is rejected
    When the enforcer reports:
      | day      | 2024-06-03 |
      | used     | 100        |
      | hostname |            |
    Then the response status is 422

  Scenario: Admin credentials cannot report status
    When the admin reports:
      | day  | 2024-06-03 |
      | used | 100        |
    Then the response status is 403

  Scenario: Unknown users cannot report
    When the enforcer reports:
      | user | other      |
      | day  | 2024-06-03 |
      | used | 100        |
    Then the response status is 404

  Scenario: Malformed reports are rejected
    When the enforcer submits report file "request-invalid.json" for user "dani"
    Then the response status is 400

  Scenario: Unknown report fields are ignored
    When the enforcer submits report file "request-unknown-field.json" for user "dani"
    Then the response status is 200
    And the status is:
      | reported_used_seconds | 100 |

  Scenario: Reports outside the window are denied
    Given a Timetower server with configuration "config/outside-window.json" at "2024-06-03T20:00:00Z"
    When the enforcer reports:
      | day  | 2024-06-03 |
      | used | 100        |
    Then the status is:
      | allowed     | false          |
      | deny_reason | outside_window |

  Scenario: Usage aggregates across hostnames
    When the enforcer reports:
      | hostname | pc-one     |
      | day      | 2024-06-03 |
      | used     | 1000       |
    And the enforcer reports:
      | hostname | pc-two     |
      | day      | 2024-06-03 |
      | used     | 500        |
    Then the status is:
      | reported_used_seconds  | 500  |
      | aggregate_used_seconds | 1500 |
