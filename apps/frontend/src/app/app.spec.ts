import { provideHttpClient } from '@angular/common/http';
import { provideHttpClientTesting } from '@angular/common/http/testing';
import { TestBed } from '@angular/core/testing';
import { provideRouter } from '@angular/router';

import { App } from './app';
import { API_CONFIG, apiConfigFor } from './core/tokens/api-config';

/**
 * The root component mounts the shell, which is the frame every page renders
 * inside: masthead, search, navigation, footer.
 *
 * The shell reads the session to decide between "Sign in" and a username, so
 * the test provides the HTTP plumbing that hangs off — not because the test
 * cares about requests, but because a component cannot be constructed without
 * what it injects.
 */
describe('App', () => {
  beforeEach(async () => {
    await TestBed.configureTestingModule({
      imports: [App],
      providers: [
        provideRouter([]),
        provideHttpClient(),
        provideHttpClientTesting(),
        { provide: API_CONFIG, useValue: apiConfigFor('http://test.local') },
      ],
    }).compileComponents();
  });

  it('creates', () => {
    const fixture = TestBed.createComponent(App);
    expect(fixture.componentInstance).toBeTruthy();
  });

  it('renders the shell around an outlet for the routed page', async () => {
    const fixture = TestBed.createComponent(App);
    await fixture.whenStable();

    const root: HTMLElement = fixture.nativeElement;
    expect(root.querySelector('app-shell')).toBeTruthy();
    expect(root.querySelector('router-outlet')).toBeTruthy();
  });

  it('offers search and the primary sections', async () => {
    const fixture = TestBed.createComponent(App);
    await fixture.whenStable();

    const root: HTMLElement = fixture.nativeElement;
    expect(root.querySelector('input[type="search"]')).toBeTruthy();

    const links = Array.from(root.querySelectorAll('nav a')).map((a) => a.textContent?.trim());
    expect(links).toContain('Transfers');
    expect(links).toContain('Clubs');
    expect(links).toContain('Competitions');

    // There is no match data, so nothing may imply a standings page exists.
    expect(links).not.toContain('Table');
    expect(links).not.toContain('Fixtures');
  });
});
