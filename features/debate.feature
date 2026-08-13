Feature: Two-agent debates
  As a user of goagentdisc
  I want to start, watch, browse, and abort debates from the TUI
  So that I can get a neutral, evidence-grounded verdict on a proposition

  Scenario: Opening the app shows the menu
    Given the app is open
    Then I should see "Start a new debate"
    And I should see "Browse archive"

  Scenario: Starting a debate from the form and watching it to a verdict
    Given the app is open
    When I open the new debate form
    And I fill in the topic "Should we ship it"
    And I submit the form
    Then I should eventually see "Verdict"
    And the debate is archived

  Scenario: Browsing an archived debate opens it read-only
    Given an archived debate titled "Demo debate" exists
    And the app is open
    When I open the archive list
    Then I should see "Archived debates"
    When I open the first archived debate
    Then I should see "Demo debate"
    And the session is read-only

  Scenario: Aborting a live debate discards it
    Given the app is open with a model that never responds
    When I open the new debate form
    And I fill in the topic "Should we ship it"
    And I submit the form
    Then I should eventually see "live"
    When I abort the debate and confirm
    Then I should eventually see "finished"
    And no debate is archived
