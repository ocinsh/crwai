"""Functions with non-ASCII identifiers (allowed by Python)."""


def café(prix):
    """Calcule le prix doublé."""
    return prix * 2


def naïve_somme(valeurs):
    return sum(valeurs)


class Société:
    """Une société avec un identifiant unicode."""

    def résumé(self):
        return "société"
