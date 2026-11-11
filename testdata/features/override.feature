Feature: Daily overrides
  Operators can adjust the current day's window and daily limit without
  editing the configuration. An override applies only to the day it was
  set and is gone the next day.

  Background:
    Given a Timetower server with configuration "config/valid.json" at "2024-06-03T10:00:00Z"

  Scenario: Setting the default override leaves the policy unchanged
    Given the current schedule revision is recorded as "initial"
    When the admin sets the override:
      | start         | 00:00 |
      | end           | 23:59 |
      | limit_minutes | 60    |
    Then the override is:
      | user          | dani            |
      | day           | 2024-06-03      |
      | limit_minutes | 60              |
      | window        | mon 00:00-23:59 |
    And the schedule revision is still "initial"
    And the schedule is:
      | daily_limit_seconds | 3600            |
      | remaining_seconds   | 3600            |
      | window              | mon 00:00-23:59 |

  Scenario: A limit override changes the schedule, status, and listing
    Given the current schedule revision is recorded as "initial"
    When the admin sets the override:
      | limit_minutes | 15 |
    Then the override is:
      | user          | dani            |
      | day           | 2024-06-03      |
      | limit_minutes | 15              |
      | window        | mon 00:00-23:59 |
    And the schedule revision differs from "initial"
    And the schedule is:
      | daily_limit_seconds | 900 |
      | remaining_seconds   | 900 |
    When the enforcer reports:
      | day  | 2024-06-03 |
      | used | 600        |
    Then the status is:
      | allowed             | true |
      | daily_limit_seconds | 900  |
      | remaining_seconds   | 300  |
    And the listing is:
      | user                | dani |
      | daily_limit_seconds | 900  |

  Scenario: A window override changes the effective window
    When the admin sets the override:
      | limit_minutes | 15 |
    And the admin sets the override:
      | start | 10:00 |
      | end   | 18:00 |
    Then the override is:
      | user          | dani            |
      | day           | 2024-06-03      |
      | limit_minutes | 15              |
      | window        | mon 10:00-18:00 |
    And the schedule is:
      | window | mon 10:00-18:00 |

  Scenario: A low limit override denies usage above it
    When the admin sets the override:
      | limit_minutes | 15 |
    And the admin sets the override:
      | start | 10:00 |
      | end   | 18:00 |
    And the admin sets the override:
      | limit_minutes | 1 |
    Then the override is:
      | user          | dani            |
      | day           | 2024-06-03      |
      | limit_minutes | 1               |
      | window        | mon 10:00-18:00 |
    When the enforcer reports:
      | day  | 2024-06-03 |
      | used | 600        |
    Then the status is:
      | allowed             | false               |
      | deny_reason         | daily_limit_reached |
      | daily_limit_seconds | 60                  |
      | remaining_seconds   | 0                   |

  Scenario: Restoring the default override restores the configured policy
    Given the current schedule revision is recorded as "initial"
    When the admin sets the override:
      | limit_minutes | 15 |
    And the admin sets the override:
      | start         | 00:00 |
      | end           | 23:59 |
      | limit_minutes | 60    |
    Then the override is:
      | user          | dani            |
      | day           | 2024-06-03      |
      | limit_minutes | 60              |
      | window        | mon 00:00-23:59 |
    And the schedule revision is still "initial"
    And the schedule is:
      | daily_limit_seconds | 3600            |
      | window              | mon 00:00-23:59 |

  Scenario: An override applies only to the current day
    When the admin sets the override:
      | limit_minutes | 15 |
    Then the override is:
      | user          | dani            |
      | day           | 2024-06-03      |
      | limit_minutes | 15              |
      | window        | mon 00:00-23:59 |
    When the server clock advances to "2024-06-04T10:00:00Z"
    And the admin requests the schedule for "dani"
    Then the schedule is:
      | daily_limit_seconds | 0 |

  Scenario: A start override collapses a split window
    Given a Timetower server with configuration "config/split-window.json" at "2024-06-03T10:00:00Z"
    When the admin sets the override:
      | start | 14:00 |
    Then the override is:
      | user          | dani            |
      | day           | 2024-06-03      |
      | limit_minutes | 60              |
      | window        | mon 14:00-19:00 |
    And the schedule is:
      | window | mon 14:00-19:00 |

  Scenario: An override creates a window on an unconfigured day
    Given a Timetower server with configuration "config/valid.json" at "2024-06-04T10:00:00Z"
    When the admin sets the override:
      | start | 10:00 |
    Then the override is:
      | user          | dani            |
      | day           | 2024-06-04      |
      | limit_minutes | 0               |
      | window        | tue 10:00-23:59 |
    When the admin sets the override:
      | start | 10:00 |
      | end   | 18:00 |
    Then the override is:
      | limit_minutes | 0               |
      | window        | tue 10:00-18:00 |
    And the schedule is:
      | window | tue 10:00-18:00 |
    When the enforcer reports:
      | day  | 2024-06-04 |
      | used | 0          |
    Then the status is:
      | inside_window | true |
      | allowed       | true |

  Scenario: Overrides survive a restart
    Given a Timetower server with state and configuration "config/valid.json" at "2024-06-03T10:00:00Z"
    When the admin sets the override:
      | limit_minutes | 15 |
    And the server is restarted
    Then the stored override for "dani" on "2024-06-03" has limit 15 minutes

  Scenario: Overrides require authentication
    When the admin submits an unauthenticated override:
      | limit_minutes | 15 |
    Then the response status is 401

  Scenario Outline: Invalid overrides are rejected
    Given a Timetower server with configuration "<config>" at "2024-06-03T10:00:00Z"
    When the admin submits a <method> override file "<request>" for user "<user>"
    Then the response status is <status>

    Examples:
      | config                 | method | user     | request                     | status |
      | config/valid.json      | PUT    | dani     | request-empty.json          | 422    |
      | config/valid.json      | PUT    | dani     | request-negative-limit.json | 422    |
      | config/valid.json      | PUT    | dani     | request-unknown-field.json  | 400    |
      | config/valid.json      | PUT    | dani     | request-bad-clock.json      | 400    |
      | config/valid.json      | PUT    | dani     | request-invalid-window.json | 422    |
      | config/valid.json      | PUT    | other    | request-limit.json          | 404    |
      | config/no-policy.json  | PUT    | nopolicy | request-limit.json          | 422    |
      | config/valid.json      | POST   | dani     | request-limit.json          | 405    |
