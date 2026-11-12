Feature: User listing
  The admin console discovers configured users through the API.

  Background:
    Given a Timetower server with configuration "config/valid.json" at "2024-06-03T10:00:00Z"

  Scenario: The admin lists the configured users
    When the admin requests the user listing
    Then the response status is 200
    And the user listing is:
      | user         | dani |
      | display_name | Dani |

  Scenario: Missing display names fall back to the account name
    Given a Timetower server with configuration "config/per-day.json" at "2024-06-03T10:00:00Z"
    When the admin requests the user listing
    Then the user listing is:
      | user         | dani |
      | display_name | dani |

  Scenario: The user listing requires admin authentication
    When the user listing is requested without credentials
    Then the response status is 401

  Scenario: Enforcer credentials cannot read the user listing
    When the enforcer requests the user listing with its key
    Then the response status is 403

  Scenario: The user listing only accepts GET
    When the admin sends a POST to the user listing
    Then the response status is 405
    And the Allow header is "GET"
