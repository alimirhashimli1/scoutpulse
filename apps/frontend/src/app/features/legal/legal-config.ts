/**
 * Who runs the site, for the legal pages.
 *
 * GDPR (Art. 13) requires the privacy policy to name the controller and a way
 * to reach them. A German Impressum (§ 5 DDG) also expects a full name and a
 * street address; add them to `operator` and `address` when they can be
 * published.
 *
 * Everything here is rendered as text on public pages.
 */
export const LEGAL = {
  /** The person or company responsible for the site (the "controller"). */
  operator: 'ScoutPulse',
  /** Postal address, one line per entry. */
  address: ['Berlin, Germany'],
  /** Where privacy and general requests go. */
  email: 'alimirhashimli@gmail.com',
  /** Country whose law and courts apply to the terms. */
  jurisdiction: 'Germany',
  /** Shown as "last updated" on every legal page. Change it when the text changes. */
  updated: '2026',
} as const;
