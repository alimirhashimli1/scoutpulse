/**
 * Who runs the site, for the legal pages.
 *
 * **Fill these in before going live.** GDPR (Art. 13) requires the privacy
 * policy to name the controller and a way to reach them, and in several EU
 * countries -- Germany and Austria most strictly -- a site must also carry a
 * legal notice (Impressum) with a real name and a postal address. Leave
 * `address` empty only if no such rule applies to you.
 *
 * Everything here is rendered as text on public pages.
 */
export const LEGAL = {
  /** The person or company responsible for the site (the "controller"). */
  operator: 'ScoutPulse operator — replace with your full name or company name',
  /** Postal address, one line per entry. Required for an Impressum. */
  address: ['Street and number — replace', 'Postcode, City — replace', 'Country — replace'],
  /** Where privacy and general requests go. Use an address you read. */
  email: 'contact@scoutpulse.example',
  /** Country whose law and courts apply to the terms. */
  jurisdiction: 'replace with your country',
  /** Shown as "last updated" on every legal page. Change it when the text changes. */
  updated: '21 November 2026',
} as const;
