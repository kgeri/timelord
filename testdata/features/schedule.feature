Feature: Schedule
  Enforcers fetch the current day's policy and usage from Timetower.

  Background:
    Given a Timetower server with configuration "config/valid.json" at "2024-06-03T10:00:00Z"

  Scenario: The schedule describes the user's day
    When the admin requests the schedule for "dani"
    Then the response status is 200
    And the response header "X-Schedule-Revision" is not empty
    And the schedule is:
      | schema_version      | 1               |
      | timezone            | UTC             |
      | user                | dani            |
      | day                 | 2024-06-03      |
      | used_seconds        | 0               |
      | remaining_seconds   | 3600            |
      | daily_limit_seconds | 3600            |
      | window              | mon 00:00-23:59 |

  Scenario: Daily limits follow the weekday
    Given a Timetower server with configuration "config/per-day.json" at "2024-06-03T10:00:00Z"
    And the current schedule revision is recorded as "monday"
    Then the schedule is:
      | daily_limit_seconds | 3600 |
      | remaining_seconds   | 3600 |
    When the server clock advances to "2024-06-04T10:00:00Z"
    Then the schedule is:
      | daily_limit_seconds | 5400 |
      | remaining_seconds   | 5400 |
    And the schedule revision differs from "monday"
    When the enforcer reports:
      | day  | 2024-06-04 |
      | used | 600        |
    Then the status is:
      | daily_limit_seconds | 5400 |
      | remaining_seconds   | 4800 |

  Scenario Outline: The schedule requires the shared key
    When the admin requests the schedule for "dani" <auth>
    Then the response status is 401

    Examples:
      | auth                |
      | without credentials |
      | with the wrong key  |

  Scenario: Unknown users have no schedule
    When the admin requests the schedule for "other"
    Then the response status is 404

  Scenario: The schedule endpoint only accepts GET
    When the admin sends a POST to the schedule for "dani"
    Then the response status is 405
    And the Allow header is "GET"
