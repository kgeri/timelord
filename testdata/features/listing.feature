Feature: Enforcer listing
  The public listing shows which enforcers have reported recently.

  Background:
    Given a Timetower server with configuration "config/valid.json" at "2024-06-03T10:00:00Z"

  Scenario: The listing is empty before any report
    When the admin requests the enforcer listing
    Then the response status is 200
    And the listing has no enforcers

  Scenario: The listing requires admin authentication
    When the enforcer listing is requested without credentials
    Then the response status is 401

  Scenario: Enforcer credentials cannot read the admin listing
    When the enforcer requests the enforcer listing with its key
    Then the response status is 403

  Scenario: A reporting enforcer appears in the listing
    When the enforcer reports:
      | day  | 2024-06-03 |
      | used | 1200       |
    And the admin requests the enforcer listing
    Then the response status is 200
    And the listing is:
      | hostname          | test-host |
      | online            | true      |
      | last_seen         | true      |
      | user              | dani      |
      | used_seconds      | 1200      |
      | remaining_seconds | 2400      |

  Scenario: An enforcer goes offline after missing heartbeats
    When the enforcer reports:
      | day  | 2024-06-03 |
      | used | 1200       |
    And the server checks for heartbeat timeouts after "4m"
    And the admin requests the enforcer listing
    Then the listing is:
      | hostname  | test-host |
      | online    | false     |
      | last_seen | true      |
